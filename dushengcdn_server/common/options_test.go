package common

import (
	"strconv"
	"sync"
	"testing"
)

func TestOptionMapAccessorsAreConcurrentSafe(t *testing.T) {
	OptionMapRWMutex.Lock()
	OptionMap = map[string]string{"key": "initial"}
	OptionMapRWMutex.Unlock()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				OptionMapRWMutex.Lock()
				OptionMap["key"] = strconv.Itoa(index*200 + j)
				OptionMapRWMutex.Unlock()
			}
		}(i)
	}
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_ = GetOptionValue("key")
				snapshot := OptionMapSnapshot()
				if _, ok := snapshot["key"]; !ok {
					t.Error("expected key to exist in option map snapshot")
					return
				}
			}
		}()
	}
	wg.Wait()
}
