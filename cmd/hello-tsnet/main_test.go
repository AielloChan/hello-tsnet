package main

import "testing"

func TestRunRejectsRetiredManagementListener(t *testing.T) {
	if code := run([]string{"-listen", "127.0.0.1:6060"}); code == 0 {
		t.Fatal("run() accepted a retired management listener")
	}
}
