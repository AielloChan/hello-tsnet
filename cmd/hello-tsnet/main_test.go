package main

import "testing"

func TestRunRejectsNonLoopbackListenAddress(t *testing.T) {
	if code := run([]string{"-listen", "0.0.0.0:6060"}); code == 0 {
		t.Fatal("run() exit code = 0 for non-loopback address")
	}
}
