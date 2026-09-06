package accounts

import (
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/db"
)

// The pool is what makes a code flow transactional, and a constructor signature is the only
// place that can insist on it.
//
// It was variadic until 2026-09-06, and this is not a hypothetical: a55cc537 is the commit
// that found BOTH production call sites — the server wiring and the integration-test helper
// — constructing the store without a pool. Begin then returned (nil, nil), so the FOR UPDATE
// read that bounds guessing to five attempts never took a lock, and "spend the code, write
// the password, bump token_version" was three unrelated statements. The service carried
// eight `if tx != nil` branches to make that legal, which is what stopped anything from
// failing. None of it ran in production for the life of that release.
//
// Written by reflection because the failure has no runtime shape to assert on: the wrong
// construction compiles, runs, and returns success.
func TestNewQueriesCodeStoreRequiresThePool(t *testing.T) {
	fn := reflect.TypeOf(NewQueriesCodeStore)
	if fn.IsVariadic() {
		t.Fatal("NewQueriesCodeStore is variadic — a caller that omits the pool compiles, " +
			"and then the code flows' serialisation and atomicity depend on how the object " +
			"was built rather than on the code (see a55cc537)")
	}
	want := []reflect.Type{
		reflect.TypeOf((*db.Queries)(nil)),
		reflect.TypeOf((*pgxpool.Pool)(nil)),
	}
	if fn.NumIn() != len(want) {
		t.Fatalf("NewQueriesCodeStore takes %d arguments, want %d", fn.NumIn(), len(want))
	}
	for i, w := range want {
		if fn.In(i) != w {
			t.Errorf("NewQueriesCodeStore argument %d is %v, want %v", i, fn.In(i), w)
		}
	}
}
