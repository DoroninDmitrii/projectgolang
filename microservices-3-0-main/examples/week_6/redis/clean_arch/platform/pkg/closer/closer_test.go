package closer

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestCloser_LIFO_Order(t *testing.T) {
	c := newCloser()

	var order []string

	c.Add("first", func(_ context.Context) error {
		order = append(order, "first")
		return nil
	})
	c.Add("second", func(_ context.Context) error {
		order = append(order, "second")
		return nil
	})
	c.Add("third", func(_ context.Context) error {
		order = append(order, "third")
		return nil
	})

	err := c.CloseAll(context.Background())
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}

	expected := []string{"third", "second", "first"}
	if len(order) != len(expected) {
		t.Fatalf("ожидалось %d вызовов, получено %d", len(expected), len(order))
	}

	for i, v := range expected {
		if order[i] != v {
			t.Errorf("позиция %d: ожидалось %q, получено %q", i, v, order[i])
		}
	}
}

func TestCloser_ReturnsFirstError(t *testing.T) {
	c := newCloser()

	errFirst := errors.New("первая ошибка")
	errSecond := errors.New("вторая ошибка")

	c.Add("ok", func(_ context.Context) error {
		return nil
	})
	// Этот вызовется вторым (LIFO), его ошибка будет первой
	c.Add("fail-second", func(_ context.Context) error {
		return errSecond
	})
	// Этот вызовется первым (LIFO)
	c.Add("fail-first", func(_ context.Context) error {
		return errFirst
	})

	err := c.CloseAll(context.Background())
	if !errors.Is(err, errFirst) {
		t.Fatalf("ожидалась первая ошибка %q, получено %q", errFirst, err)
	}
}

func TestCloser_AllFunctionsCalledDespiteErrors(t *testing.T) {
	c := newCloser()

	var called []string

	c.Add("a", func(_ context.Context) error {
		called = append(called, "a")
		return nil
	})
	c.Add("b", func(_ context.Context) error {
		called = append(called, "b")
		return errors.New("ошибка b")
	})
	c.Add("c", func(_ context.Context) error {
		called = append(called, "c")
		return nil
	})

	_ = c.CloseAll(context.Background())

	if len(called) != 3 {
		t.Fatalf("ожидалось 3 вызова, получено %d: %v", len(called), called)
	}
}

func TestCloser_CloseAllOnce(t *testing.T) {
	c := newCloser()

	callCount := 0
	c.Add("resource", func(_ context.Context) error {
		callCount++
		return nil
	})

	_ = c.CloseAll(context.Background())
	_ = c.CloseAll(context.Background())
	_ = c.CloseAll(context.Background())

	if callCount != 1 {
		t.Fatalf("ожидался 1 вызов, получено %d", callCount)
	}
}

func TestCloser_EmptyCloser(t *testing.T) {
	c := newCloser()

	err := c.CloseAll(context.Background())
	if err != nil {
		t.Fatalf("ожидалась nil-ошибка для пустого closer, получено %v", err)
	}
}

func TestCloser_RespectsContextCancellation(t *testing.T) {
	c := newCloser()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // отменяем сразу

	var receivedCtx context.Context
	c.Add("resource", func(ctx context.Context) error {
		receivedCtx = ctx
		return ctx.Err()
	})

	err := c.CloseAll(ctx)
	if nil == err {
		t.Fatal("ожидалась ошибка контекста, получено nil")
	}

	if nil == receivedCtx.Err() {
		t.Fatal("ожидалось, что отменённый контекст будет передан в функцию закрытия")
	}
}

func TestCloser_ConcurrentAdd(t *testing.T) {
	c := newCloser()

	var wg sync.WaitGroup
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Add("resource", func(_ context.Context) error {
				return nil
			})
		}()
	}
	wg.Wait()

	// Проверяем что все 100 добавились
	c.mu.Lock()
	count := len(c.funcs)
	c.mu.Unlock()

	if count != 100 {
		t.Fatalf("ожидалось 100 функций, получено %d", count)
	}
}

func TestCloser_ContextTimeout(t *testing.T) {
	c := newCloser()

	c.Add("slow", func(ctx context.Context) error {
		select {
		case <-time.After(5 * time.Second):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := c.CloseAll(ctx)
	elapsed := time.Since(start)

	if nil == err {
		t.Fatal("ожидалась ошибка таймаута, получено nil")
	}

	if elapsed > 1*time.Second {
		t.Fatalf("ожидался быстрый таймаут, но заняло %v", elapsed)
	}
}
