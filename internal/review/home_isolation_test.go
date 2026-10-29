package review

import (
	"os"
	"testing"

	"github.com/BSH2409/hunkyard/internal/testutil"
)

func TestMain(m *testing.M) { os.Exit(testutil.RunWithTempHome(m)) }
