package dnsworker

import (
	"bufio"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type OperatorCIDRMatcher struct {
	ranges []operatorCIDRRange
}

type operatorCIDRRange struct {
	start    *big.Int
	end      *big.Int
	bits     int
	operator string
}

var operatorCIDRFileOperators = map[string]string{
	"chinanet":     "cn-telecom",
	"chinatelecom": "cn-telecom",
	"telecom":      "cn-telecom",
	"unicom":       "cn-unicom",
	"chinaunicom":  "cn-unicom",
	"cmcc":         "cn-mobile",
	"chinamobile":  "cn-mobile",
	"mobile":       "cn-mobile",
	"tietong":      "cn-mobile",
	"cernet":       "cernet",
	"cernet2":      "cernet",
	"edu":          "cernet",
	"cstnet":       "cstnet",
	"drpeng":       "drpeng",
	"googlecn":     "googlecn",
	"cbn":          "cn-broadcast",
	"chinabtn":     "cn-broadcast",
	"broadcast":    "cn-broadcast",
}

func LoadOperatorCIDRMatcher(path string) (*OperatorCIDRMatcher, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return &OperatorCIDRMatcher{}, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	files := []operatorCIDRFile{}
	if info.IsDir() {
		entries, err := os.ReadDir(path)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			operator := operatorFromCIDRFilename(entry.Name())
			if operator == "" {
				continue
			}
			files = append(files, operatorCIDRFile{
				path:     filepath.Join(path, entry.Name()),
				operator: operator,
			})
		}
	} else {
		operator := operatorFromCIDRFilename(filepath.Base(path))
		if operator == "" {
			operator = "cn-telecom"
		}
		files = append(files, operatorCIDRFile{path: path, operator: operator})
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no supported operator CIDR files found in %s", path)
	}
	sort.SliceStable(files, func(i, j int) bool {
		return files[i].path < files[j].path
	})
	ranges := make([]operatorCIDRRange, 0)
	for _, file := range files {
		loaded, err := loadOperatorCIDRFile(file.path, file.operator)
		if err != nil {
			return nil, err
		}
		ranges = append(ranges, loaded...)
	}
	sort.SliceStable(ranges, func(i, j int) bool {
		if ranges[i].bits != ranges[j].bits {
			return ranges[i].bits < ranges[j].bits
		}
		if cmp := ranges[i].start.Cmp(ranges[j].start); cmp != 0 {
			return cmp < 0
		}
		if cmp := ranges[i].end.Cmp(ranges[j].end); cmp != 0 {
			return cmp < 0
		}
		return ranges[i].operator < ranges[j].operator
	})
	return &OperatorCIDRMatcher{ranges: ranges}, nil
}

type operatorCIDRFile struct {
	path     string
	operator string
}

func loadOperatorCIDRFile(path string, operator string) ([]operatorCIDRRange, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	ranges := make([]operatorCIDRRange, 0)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		text := strings.TrimSpace(scanner.Text())
		if text == "" || strings.HasPrefix(text, "#") || strings.HasPrefix(text, "//") {
			continue
		}
		if before, _, ok := strings.Cut(text, "#"); ok {
			text = strings.TrimSpace(before)
		}
		if before, _, ok := strings.Cut(text, ","); ok {
			text = strings.TrimSpace(before)
		}
		if text == "" {
			continue
		}
		item, err := parseOperatorCIDR(text, operator)
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, lineNumber, err)
		}
		ranges = append(ranges, item)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return ranges, nil
}

func parseOperatorCIDR(value string, operator string) (operatorCIDRRange, error) {
	text := strings.TrimSpace(value)
	if text == "" {
		return operatorCIDRRange{}, fmt.Errorf("empty CIDR")
	}
	if !strings.Contains(text, "/") {
		ip := net.ParseIP(text)
		if ip == nil {
			return operatorCIDRRange{}, fmt.Errorf("invalid IP or CIDR %q", value)
		}
		if ip.To4() != nil {
			text = ip.String() + "/32"
		} else {
			text = ip.String() + "/128"
		}
	}
	ip, network, err := net.ParseCIDR(text)
	if err != nil || network == nil {
		return operatorCIDRRange{}, fmt.Errorf("invalid CIDR %q", value)
	}
	startIP := ip.Mask(network.Mask)
	start, bits := ipToSortableInt(startIP)
	if start == nil {
		return operatorCIDRRange{}, fmt.Errorf("invalid CIDR IP %q", value)
	}
	ones, maskBits := network.Mask.Size()
	if ones < 0 || maskBits != bits {
		return operatorCIDRRange{}, fmt.Errorf("invalid CIDR mask %q", value)
	}
	size := new(big.Int).Lsh(big.NewInt(1), uint(bits-ones))
	end := new(big.Int).Add(start, size)
	end.Sub(end, big.NewInt(1))
	return operatorCIDRRange{
		start:    start,
		end:      end,
		bits:     bits,
		operator: normalizeOperator(operator),
	}, nil
}

func (m *OperatorCIDRMatcher) Count() int {
	if m == nil {
		return 0
	}
	return len(m.ranges)
}

func (m *OperatorCIDRMatcher) Lookup(ip net.IP) string {
	if m == nil || len(m.ranges) == 0 || ip == nil {
		return ""
	}
	value, bits := ipToSortableInt(ip)
	if value == nil {
		return ""
	}
	first := sort.Search(len(m.ranges), func(i int) bool {
		return m.ranges[i].bits >= bits
	})
	if first >= len(m.ranges) || m.ranges[first].bits != bits {
		return ""
	}
	last := sort.Search(len(m.ranges), func(i int) bool {
		return m.ranges[i].bits > bits
	})
	index := first + sort.Search(last-first, func(i int) bool {
		return m.ranges[first+i].start.Cmp(value) > 0
	}) - 1
	for index >= first {
		item := m.ranges[index]
		if item.start.Cmp(value) <= 0 && item.end.Cmp(value) >= 0 {
			return item.operator
		}
		if item.end.Cmp(value) < 0 {
			break
		}
		index--
	}
	return ""
}

func operatorFromCIDRFilename(name string) string {
	base := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), filepath.Ext(name))
	base = strings.NewReplacer("-", "", "_", "", ".", "").Replace(base)
	if strings.HasSuffix(base, "6") {
		base = strings.TrimSuffix(base, "6")
	}
	switch base {
	case "", "china", "all", "stat", "statistics":
		return ""
	}
	if operator, ok := operatorCIDRFileOperators[base]; ok {
		return operator
	}
	return ""
}

func ipToSortableInt(ip net.IP) (*big.Int, int) {
	if ip == nil {
		return nil, 0
	}
	if ipv4 := ip.To4(); ipv4 != nil {
		return new(big.Int).SetBytes(ipv4), 32
	}
	ipv6 := ip.To16()
	if ipv6 == nil {
		return nil, 0
	}
	return new(big.Int).SetBytes(ipv6), 128
}
