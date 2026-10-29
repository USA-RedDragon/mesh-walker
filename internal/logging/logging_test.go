package logging

import (
	"bytes"
	"strings"
	"testing"
)

func TestSplitByLevel(t *testing.T) {
	var info, errOut bytes.Buffer
	log := New(&info, &errOut).With("run", 1)
	log.Debug("hidden")
	log.Info("Starting walk", "startingNode", "N0CALL")
	log.Warn("careful")
	log.Error("Error fetching data", "node", "x")

	if got := info.String(); !strings.Contains(got, `level=INFO msg="Starting walk" run=1 startingNode=N0CALL`) || strings.Contains(got, "ERROR") || strings.Contains(got, "WARN") || strings.Contains(got, "hidden") {
		t.Errorf("info output: %q", got)
	}
	if got := errOut.String(); !strings.Contains(got, "level=WARN msg=careful") || !strings.Contains(got, `level=ERROR msg="Error fetching data" run=1 node=x`) || strings.Contains(got, "INFO") {
		t.Errorf("err output: %q", got)
	}
	if strings.Contains(info.String()+errOut.String(), "time=") {
		t.Error("time attribute was not removed")
	}
}
