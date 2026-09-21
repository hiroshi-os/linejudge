package worker

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/hiroshi-os/linejudge/internal/githubx"
	"github.com/hiroshi-os/linejudge/internal/pipeline"
	"github.com/hiroshi-os/linejudge/internal/store"
	"github.com/hiroshi-os/linejudge/internal/types"
)

type Worker struct {
	Store  *store.Store
	Runner pipeline.Runner
}

func New(st *store.Store) *Worker {
	return &Worker{
		Store: st,
		Runner: pipeline.Runner{
			Publisher: githubx.NewPublisher(),
		},
	}
}

func (w *Worker) Loop(ctx context.Context) {
	t := time.NewTicker(200 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := w.tick(ctx); err != nil {
				log.Printf("worker: %v", err)
			}
		}
	}
}

func (w *Worker) tick(ctx context.Context) error {
	jobID, reviewID, ok, err := w.Store.ClaimJob(ctx)
	if err != nil || !ok {
		return err
	}
	if err := w.process(ctx, reviewID); err != nil {
		n, _ := w.Store.JobAttempts(ctx, jobID)
		if n >= 5 {
			_ = w.Store.FinishJob(ctx, jobID, "failed", err.Error())
			_ = w.Store.MarkDone(ctx, reviewID, types.StatusFailed, "", err.Error(), 0, 0)
			return err
		}
		_ = w.Store.RequeueJob(ctx, jobID, time.Duration(n)*time.Second, err.Error())
		return err
	}
	return w.Store.FinishJob(ctx, jobID, "done", "")
}

func (w *Worker) process(ctx context.Context, reviewID string) error {
	rev, diff, err := w.Store.GetReview(ctx, reviewID)
	if err != nil {
		return err
	}
	if err := w.Store.MarkRunning(ctx, reviewID); err != nil {
		return err
	}
	cfg, err := w.Store.GetConfig(ctx)
	if err != nil {
		return err
	}
	res, err := w.Runner.Run(ctx, rev, diff, cfg)
	for _, ev := range res.Events {
		_ = w.Store.AddEvent(ctx, ev)
	}
	if err != nil {
		_ = w.Store.MarkDone(ctx, reviewID, types.StatusFailed, "", err.Error(), 0, 0)
		return err
	}
	if err := w.Store.SetPacked(ctx, reviewID, res.Packed); err != nil {
		return err
	}
	if err := w.Store.ReplaceFindings(ctx, reviewID, res.Findings); err != nil {
		return err
	}
	if err := w.Store.ReplaceComments(ctx, reviewID, res.Comments); err != nil {
		return err
	}
	return w.Store.MarkDone(ctx, reviewID, types.StatusCompleted, res.Summary, "", len(res.Findings), len(res.Comments))
}

func (w *Worker) Drain(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		jobID, reviewID, ok, err := w.Store.ClaimJob(ctx)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		if err := w.process(ctx, reviewID); err != nil {
			_ = w.Store.FinishJob(ctx, jobID, "failed", err.Error())
			return err
		}
		if err := w.Store.FinishJob(ctx, jobID, "done", ""); err != nil {
			return err
		}
	}
	return fmt.Errorf("drain timeout")
}
