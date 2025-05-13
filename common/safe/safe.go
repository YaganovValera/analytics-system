package safe

import (
	"context"
	"sync"

	"go.uber.org/zap"
)

// Group — аналог errgroup.Group с защитой от panic.
type Group struct {
	wg     sync.WaitGroup
	cancel context.CancelFunc
	ctx    context.Context
	log    *zap.Logger
}

// New создает группу с контекстом и логгером.
func New(ctx context.Context, log *zap.Logger) *Group {
	ctx, cancel := context.WithCancel(ctx)
	return &Group{
		ctx:    ctx,
		cancel: cancel,
		log:    log.Named("safe"),
	}
}

// Go запускает защищённую goroutine.
func (g *Group) Go(fn func(ctx context.Context) error) {
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		defer g.recoverPanic()
		if err := fn(g.ctx); err != nil {
			g.log.Error("goroutine error", zap.Error(err))
			g.cancel()
		}
	}()
}

// Wait блокирует до завершения всех goroutine.
func (g *Group) Wait() {
	g.wg.Wait()
}

// Context возвращает связанный контекст.
func (g *Group) Context() context.Context {
	return g.ctx
}

// recoverPanic ловит панику и логирует её.
func (g *Group) recoverPanic() {
	if r := recover(); r != nil {
		g.log.Error("panic recovered", zap.Any("error", r))
		g.cancel()
	}
}
