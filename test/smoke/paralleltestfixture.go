package smoke

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
)

type (
	parallelTestFixture struct {
		T                  *testing.T
		Matrix             matrixDef
		GetAddrs           func(int) []string
		testNames          map[string]struct{}
		testNamesMu        sync.RWMutex
		testNamesPassed    map[string]struct{}
		testNamesPassedMu  sync.Mutex
		testNamesSkipped   map[string]struct{}
		testNamesSkippedMu sync.Mutex
		testNamesFailed    map[string]struct{}
		testNamesFailedMu  sync.Mutex
	}

	parallelTestFixtureSet struct {
		GetAddrs func(int) []string
		mu       sync.Mutex
		fixtures map[string]*parallelTestFixture
	}
)

func newParallelTestFixtureSet() *parallelTestFixtureSet {
	if err := stopPIDs(); err != nil {
		panic(err)
	}
	getAddrs := func(n int) []string {
		return freePortAddrs("127.0.0.1", n, 49152, 65535)
	}
	return &parallelTestFixtureSet{
		GetAddrs: getAddrs,
		fixtures: map[string]*parallelTestFixture{},
	}
}

func (pfs *parallelTestFixtureSet) newParallelTestFixture(t *testing.T, m matrixDef) *parallelTestFixture {
	if flags.printMatrix {
		matrix := m.FixtureConfigs()
		for _, m := range matrix {
			fmt.Printf("%s/%s\n", t.Name(), m.Desc)
		}
		t.Skip("Just printing test matrix (-ls-matrix flag set)")
	}
	rtLog("Registering %s", t.Name())
	t.Helper()
	t.Parallel()
	rtLog("Running     %s", t.Name())
	pf := &parallelTestFixture{
		T:                t,
		Matrix:           m,
		GetAddrs:         pfs.GetAddrs,
		testNames:        map[string]struct{}{},
		testNamesPassed:  map[string]struct{}{},
		testNamesSkipped: map[string]struct{}{},
		testNamesFailed:  map[string]struct{}{},
	}
	pfs.mu.Lock()
	defer pfs.mu.Unlock()
	pfs.fixtures[t.Name()] = pf
	return pf
}

func (pf *parallelTestFixture) recordTestStarted(t *testing.T) {
	t.Helper()
	name := t.Name()
	pf.testNamesMu.Lock()
	defer pf.testNamesMu.Unlock()
	if _, ok := pf.testNames[name]; ok {
		t.Fatalf("duplicate test name: %q", name)
	}
	pf.testNames[name] = struct{}{}
}

// PTest is a test to run in parallel.
type PTest struct {
	Name string
	Test func(*testing.T, *testFixture)
}

// RunMatrix runs the provided PTests in parallel, once for each combination of
// the matrix passed to newParallelTestFixture.
func (pf *parallelTestFixture) RunMatrix(tests ...PTest) {
	for _, c := range pf.Matrix.FixtureConfigs() {
		pf.T.Run(c.Desc, func(t *testing.T) {
			c := c
			t.Parallel()
			for _, pt := range tests {
				pt := pt
				t.Run(pt.Name, func(t *testing.T) {
					f := pf.newIsolatedFixture(t, c)
					defer f.Teardown(t)
					defer f.reportStatus(t)
					pt.Test(t, f)
				})
			}
		})
	}
}

func (pf *parallelTestFixture) recordTestStatus(t *testing.T) {
	t.Helper()
	name := t.Name()
	pf.testNamesMu.RLock()
	_, started := pf.testNames[name]
	pf.testNamesMu.RUnlock()

	statusString := "UNKNOWN"
	status := &statusString
	defer func() { rtLog("Finished running %s: %s", name, *status) }()

	if !started {
		t.Fatalf("test %q reported as finished, but not started", name)
		*status = "ERROR: Not Started"
		return
	}
	switch {
	default:
		*status = "PASSED"
		pf.testNamesPassedMu.Lock()
		pf.testNamesPassed[name] = struct{}{}
		pf.testNamesPassedMu.Unlock()
		return
	case t.Skipped():
		*status = "SKIPPED"
		pf.testNamesSkippedMu.Lock()
		pf.testNamesSkipped[name] = struct{}{}
		pf.testNamesSkippedMu.Unlock()
		return
	case t.Failed():
		*status = "FAILED"
		pf.testNamesFailedMu.Lock()
		pf.testNamesFailed[name] = struct{}{}
		pf.testNamesFailedMu.Unlock()
		return
	}
}

// PrintSummary prints a summary of tests run by top-level test and as a sum
// total. It reports tests failed, skipped, passed, and missing (when a test has
// failed to report back any status, which should not happen under normal
// circumstances.
func (pfs *parallelTestFixtureSet) PrintSummary() {
	var total, passed, skipped, failed, missing []string
	for _, pf := range pfs.fixtures {
		t, p, s, f, m := pf.printSummary()
		total = append(total, t...)
		passed = append(passed, p...)
		skipped = append(skipped, s...)
		failed = append(failed, f...)
		missing = append(missing, m...)
	}

	if len(failed) != 0 {
		fmt.Printf("These tests failed:\n")
		for _, n := range failed {
			fmt.Printf("> %s\n", n)
		}
	}

	summary := fmt.Sprintf("Summary: %d failed; %d skipped; %d passed; %d missing (total %d)",
		len(failed), len(skipped), len(passed), len(missing), len(total))
	fmt.Fprintln(os.Stdout, summary)
}

func testNamesSlice(m map[string]struct{}) []string {
	var s, i = make([]string, len(m)), 0
	for n := range m {
		s[i] = n
		i++
	}
	return s
}

func (pf *parallelTestFixture) printSummary() (total, passed, skipped, failed, missing []string) {
	t := pf.T
	t.Helper()
	total = testNamesSlice(pf.testNames)
	passed = testNamesSlice(pf.testNamesPassed)
	skipped = testNamesSlice(pf.testNamesSkipped)
	failed = testNamesSlice(pf.testNamesFailed)

	summary := fmt.Sprintf("%s summary: %d failed; %d skipped; %d passed (total %d)",
		t.Name(), len(failed), len(skipped), len(passed), len(total))
	t.Log(summary)
	fmt.Fprintln(os.Stdout, summary)

	var missingTests []string
	missingCount := len(total) - (len(passed) + len(failed) + len(skipped))
	if missingCount != 0 {
		for t := range pf.testNamesPassed {
			delete(pf.testNames, t)
		}
		for t := range pf.testNamesSkipped {
			delete(pf.testNames, t)
		}
		for t := range pf.testNamesFailed {
			delete(pf.testNames, t)
		}
		for t := range pf.testNames {
			missingTests = append(missingTests, t)
		}
		rtLog("Warning! Some tests did not report status: %s", strings.Join(missingTests, ", "))
	}
	return total, passed, skipped, failed, missingTests
}

func (pf *parallelTestFixture) newIsolatedFixture(t *testing.T, fcfg fixtureConfig) *testFixture {
	t.Helper()
	pf.recordTestStarted(t)
	envDesc := getEnvDesc()
	return newTestFixture(t, envDesc, pf, pf.GetAddrs, fcfg)
}
