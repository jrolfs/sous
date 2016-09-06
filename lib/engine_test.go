package sous

import (
	"testing"
)

func TestEngine_ParseSourceLocation_noHosts(t *testing.T) {
	e := &Engine{}
	expected := `source location not recognised: ""`
	sl, actualErr := e.ParseSourceLocation("")
	if (sl != SourceLocation{}) {
		t.Errorf("got non-zero source location: %#v", sl)
	}
	if actualErr == nil {
		t.Fatalf("got nil; want error %q", expected)
	}
	actual := actualErr.Error()
	if actual != expected {
		t.Errorf("got error %q; want %q", actual, expected)
	}
}

func TestEngine_ParseSourceLocation_genericHost(t *testing.T) {
	e := &Engine{
		SourceHosts: []SourceHost{GenericHost{}},
	}
	expected := SourceLocation{Repo: "hello"}
	actual, err := e.ParseSourceLocation("hello")
	if err != nil {
		t.Fatal(err)
	}
	if actual != expected {
		t.Errorf("got:\n%#v; want:\n%#v", actual, expected)
	}
}