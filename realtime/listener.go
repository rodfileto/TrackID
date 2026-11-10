package realtime

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	listenerMinBackoff = time.Second
	listenerMaxBackoff = 30 * time.Second
	listenerConnect    = 10 * time.Second
	// listenerHeartbeat bounds how long a silently dead connection can go unnoticed.
	listenerHeartbeat = time.Minute
)

// Listen follows a Postgres NOTIFY channel until ctx ends, calling handle with each payload, so
// events raised in the worker reach this process. It holds one dedicated connection, which must
// not go through PgBouncer in transaction mode, and reconnects with backoff. reconnected runs
// after each reconnection, since anything sent during the gap is gone.
func Listen(ctx context.Context, databaseURL, channel string, handle func(context.Context, string), reconnected func()) {
	wait := listenerMinBackoff
	connectedBefore := false
	for ctx.Err() == nil {
		err := listenOnce(ctx, databaseURL, channel, handle, func() {
			wait = listenerMinBackoff
			if connectedBefore && reconnected != nil {
				reconnected()
			}
			connectedBefore = true
		})
		if ctx.Err() != nil {
			return
		}
		log.Printf("realtime: listener on %s lost, retrying in %s: %v", channel, wait, err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait = min(wait*2, listenerMaxBackoff)
	}
}

func listenOnce(ctx context.Context, databaseURL, channel string, handle func(context.Context, string), connected func()) error {
	connectCtx, cancel := context.WithTimeout(ctx, listenerConnect)
	defer cancel()
	conn, err := pgx.Connect(connectCtx, databaseURL)
	if err != nil {
		return err
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), listenerConnect)
		defer cancel()
		conn.Close(closeCtx)
	}()
	if _, err := conn.Exec(connectCtx, "LISTEN "+pgx.Identifier{channel}.Sanitize()); err != nil {
		return err
	}
	connected()

	for {
		waitCtx, cancelWait := context.WithTimeout(ctx, listenerHeartbeat)
		notification, err := conn.WaitForNotification(waitCtx)
		cancelWait()
		if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
			pingCtx, cancelPing := context.WithTimeout(ctx, listenerConnect)
			err := conn.Ping(pingCtx)
			cancelPing()
			if err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		handle(ctx, notification.Payload)
	}
}
