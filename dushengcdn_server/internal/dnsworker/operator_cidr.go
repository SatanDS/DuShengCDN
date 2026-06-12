package dnsworker

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type OperatorCIDRMatcher struct {
	ranges []operatorCIDRRange
}

type operatorCIDRRange struct {
	start    netip.Addr
	end      netip.Addr
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
		if cmp := compareOperatorCIDRAddr(ranges[i].start, ranges[j].start); cmp != 0 {
			return cmp < 0
		}
		if cmp := compareOperatorCIDRAddr(ranges[i].end, ranges[j].end); cmp != 0 {
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
	var prefix netip.Prefix
	var err error
	if strings.Contains(text, "/") {
		prefix, err = netip.ParsePrefix(text)
	} else {
		var addr netip.Addr
		addr, err = netip.ParseAddr(text)
		if err == nil {
			addr = addr.Unmap()
			bits := 128
			if addr.Is4() {
				bits = 32
			}
			prefix = netip.PrefixFrom(addr, bits)
		}
	}
	if err != nil || !prefix.IsValid() {
		return operatorCIDRRange{}, fmt.Errorf("invalid CIDR %q", value)
	}
	start, end, bits, ok := operatorCIDRPrefixRange(prefix)
	if !ok {
		return operatorCIDRRange{}, fmt.Errorf("invalid CIDR IP %q", value)
	}
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
	value, bits, ok := operatorCIDRAddrFromIP(ip)
	if !ok {
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
		return compareOperatorCIDRAddr(m.ranges[first+i].start, value) > 0
	}) - 1
	for index >= first {
		item := m.ranges[index]
		if compareOperatorCIDRAddr(item.start, value) <= 0 && compareOperatorCIDRAddr(item.end, value) >= 0 {
			return item.operator
		}
		if compareOperatorCIDRAddr(item.end, value) < 0 {
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

func operatorCIDRAddrFromIP(ip net.IP) (netip.Addr, int, bool) {
	if ip == nil {
		return netip.Addr{}, 0, false
	}
	if ipv4 := ip.To4(); ipv4 != nil {
		var bytes [4]byte
		copy(bytes[:], ipv4)
		return netip.AddrFrom4(bytes), 32, true
	}
	ipv6 := ip.To16()
	if ipv6 == nil {
		return netip.Addr{}, 0, false
	}
	var bytes [16]byte
	copy(bytes[:], ipv6)
	return netip.AddrFrom16(bytes), 128, true
}

func operatorCIDRPrefixRange(prefix netip.Prefix) (netip.Addr, netip.Addr, int, bool) {
	if !prefix.IsValid() {
		return netip.Addr{}, netip.Addr{}, 0, false
	}
	prefix = netip.PrefixFrom(prefix.Addr().Unmap(), prefix.Bits()).Masked()
	if !prefix.IsValid() {
		return netip.Addr{}, netip.Addr{}, 0, false
	}
	addr := prefix.Addr()
	if addr.Is4() {
		startBytes := addr.As4()
		startValue := binary.BigEndian.Uint32(startBytes[:])
		hostBits := 32 - prefix.Bits()
		endValue := startValue
		if hostBits >= 32 {
			endValue = ^uint32(0)
		} else if hostBits > 0 {
			endValue |= uint32(1<<hostBits) - 1
		}
		var endBytes [4]byte
		binary.BigEndian.PutUint32(endBytes[:], endValue)
		return addr, netip.AddrFrom4(endBytes), 32, true
	}
	if !addr.Is6() {
		return netip.Addr{}, netip.Addr{}, 0, false
	}
	endBytes := addr.As16()
	ones := prefix.Bits()
	if ones < 0 || ones > 128 {
		return netip.Addr{}, netip.Addr{}, 0, false
	}
	fullBytes := ones / 8
	remainingBits := ones % 8
	if remainingBits > 0 && fullBytes < len(endBytes) {
		endBytes[fullBytes] |= byte(0xff >> remainingBits)
		fullBytes++
	}
	for i := fullBytes; i < len(endBytes); i++ {
		endBytes[i] = 0xff
	}
	return addr, netip.AddrFrom16(endBytes), 128, true
}

func compareOperatorCIDRAddr(a netip.Addr, b netip.Addr) int {
	if a == b {
		return 0
	}
	if a.Less(b) {
		return -1
	}
	return 1
}
