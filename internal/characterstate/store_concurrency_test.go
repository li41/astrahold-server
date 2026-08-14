package characterstate

import (
	"errors"
	"sync"
	"testing"
)

func TestStoreConcurrentCreateAllowsExactlyOneWriter(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	identity := trusted(t, "character:race")
	snapshot := testSnapshot()

	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := store.Save(identity, 0, snapshot)
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)

	successes := 0
	conflicts := 0
	for err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrRevisionConflict):
			conflicts++
		default:
			t.Fatalf("unexpected err=%v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d", successes, conflicts)
	}
	loaded, ok, err := store.Load(identity)
	if err != nil || !ok || loaded.Revision != 1 {
		t.Fatalf("loaded=%#v ok=%v err=%v", loaded, ok, err)
	}
}
