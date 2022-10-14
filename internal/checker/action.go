package checker

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/vektah/gqlparser/v2/ast"

	"github.com/gqlgo/gqlanalysis"
)

type action struct {
	once sync.Once

	a        *gqlanalysis.Analyzer
	deps     []*action
	schema   *ast.Schema
	queries  []*ast.QueryDocument
	comments []*gqlanalysis.Comment

	isroot      bool
	pass        *gqlanalysis.Pass
	err         error
	result      interface{}
	diagnostics []*gqlanalysis.Diagnostic
}

func (act *action) String() string {
	return act.a.Name
}

func (act *action) report(d *gqlanalysis.Diagnostic) {
	act.diagnostics = append(act.diagnostics, d)
}

func (act *action) exec() {
	act.once.Do(act.execOnce)
}

func (act *action) execOnce() {
	execAll(act.deps)

	var failed []string
	for _, dep := range act.deps {
		if dep.err != nil {
			failed = append(failed, dep.String())
		}
	}
	if failed != nil {
		sort.Strings(failed)
		act.err = fmt.Errorf("failed prerequisites: %s", strings.Join(failed, ", "))
		return
	}

	inputs := make(map[*gqlanalysis.Analyzer]interface{})
	for _, a := range act.deps {
		if a.result != nil {
			inputs[a.a] = a.result
		}
	}

	pass := &gqlanalysis.Pass{
		Analyzer: act.a,
		Schema:   act.schema,
		Queries:  act.queries,
		Comments: act.comments,
		Report:   act.report,
		ResultOf: inputs,
	}
	act.pass = pass

	var err error
	act.result, err = pass.Analyzer.Run(pass)
	if err == nil {
		if got, want := reflect.TypeOf(act.result), pass.Analyzer.ResultType; got != want {
			err = fmt.Errorf(
				"internal error: analyzer %s returned a result of type %v, but declared ResultType %v", pass.Analyzer, got, want)
		}
	}
	act.err = err
}

func execAll(actions []*action) {
	sequential := false // for debug
	var wg sync.WaitGroup
	for _, act := range actions {
		wg.Add(1)
		work := func(act *action) {
			act.exec()
			wg.Done()
		}
		if sequential {
			work(act)
		} else {
			go work(act)
		}
	}
	wg.Wait()
}
