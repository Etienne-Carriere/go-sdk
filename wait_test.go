package transloadit

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestWaitForAssembly(t *testing.T) {
	t.Parallel()

	client := setup(t)

	assembly := NewAssembly()

	assembly.AddStep("import", map[string]interface{}{
		"robot": "/http/import",
		"url":   "http://mirror.nl.leaseweb.net/speedtest/100mb.bin",
	})

	info, err := client.StartAssembly(ctx, assembly)
	if err != nil {
		t.Fatal(err)
	}

	if info.AssemblyURL == "" {
		t.Fatal("response doesn't contain assembly_url")
	}

	finishedInfo, err := client.WaitForAssembly(ctx, info)
	if err != nil {
		t.Fatal(err)
	}

	// Assembly completed
	if finishedInfo.AssemblyID != info.AssemblyID {
		t.Fatal("unmatching assembly ids")
	}
}

func TestWaitForAssembly_Cancel(t *testing.T) {
	t.Parallel()
	client := setup(t)

	ctx, cancel := context.WithTimeout(ctx, time.Millisecond)
	defer cancel()

	_, err := client.WaitForAssembly(ctx, &AssemblyInfo{
		AssemblySSLURL: "https://api2.transloadit.com/assemblies/foo",
	})
	// Go 1.8 and Go 1.7 have different error messages if a request get canceled.
	// Therefore we test for both cases.
	if !strings.Contains(err.Error(), "context deadline exceeded") && !strings.Contains(err.Error(), "request canceled") {
		t.Fatalf("operation's deadline should be exceeded: %s", err)
	}
}
