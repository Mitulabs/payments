package mitu

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

const (
	testAPIKey = "sk_test_abcdefghijklmnop"
	liveAPIKey = "sk_live_abcdefghijklmnop"
)

func TestMain(m *testing.M) {
	code := m.Run()
	if code == 0 {
		if leaked := leakedGoroutines(); leaked != "" {
			fmt.Fprintf(os.Stderr, "goroutines leaked by the package:\n\n%s\n", leaked)
			code = 1
		}
	}
	os.Exit(code)
}

// leakedGoroutines returns the stacks of goroutines still running package
// code after every test has finished, allowing a moment for ones that are
// already on their way out.
func leakedGoroutines() string {
	var leaked []string
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		buf := make([]byte, 1<<20)
		buf = buf[:runtime.Stack(buf, true)]
		leaked = leaked[:0]
		for stack := range strings.SplitSeq(string(buf), "\n\n") {
			if (strings.Contains(stack, "github.com/mitulabs/payments/go.") || strings.Contains(stack, "net/http/httptest.")) &&
				!strings.Contains(stack, "TestMain") {
				leaked = append(leaked, stack)
			}
		}
		if len(leaked) == 0 {
			return ""
		}
	}
	return strings.Join(leaked, "\n\n")
}
