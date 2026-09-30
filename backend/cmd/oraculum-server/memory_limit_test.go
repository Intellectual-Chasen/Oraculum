package main

import (
	"io/fs"
	"testing"
)

func TestCgroupMemoryMaxTakesSmallestLimitUpToRoot(t *testing.T) {
	files := map[string]string{
		"/sys/fs/cgroup/user.slice/app.scope/memory.max": "4294967296\n",
		"/sys/fs/cgroup/user.slice/memory.max":           "max\n",
		"/sys/fs/cgroup/memory.max":                      "8589934592\n",
	}
	readFile := func(name string) ([]byte, error) {
		content, found := files[name]
		if !found {
			return nil, fs.ErrNotExist
		}
		return []byte(content), nil
	}
	limit, found := cgroupMemoryMax("0::/user.slice/app.scope\n", readFile)
	if !found || limit != 4294967296 {
		t.Fatalf("limit = %d, found = %v", limit, found)
	}
	files["/sys/fs/cgroup/user.slice/app.scope/memory.max"] = "max\n"
	delete(files, "/sys/fs/cgroup/memory.max")
	if limit, found := cgroupMemoryMax("0::/user.slice/app.scope\n", readFile); found {
		t.Fatalf("unlimited hierarchy returned %d", limit)
	}
	if _, found := cgroupMemoryMax("1:name=systemd:/x\n", readFile); found {
		t.Fatal("a cgroup v1 membership returned a limit")
	}
}
