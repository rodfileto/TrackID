package api

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"

	"github.com/rodfileto/trackid/auth"
	"github.com/rodfileto/trackid/realtime"
)

// RealtimeTicketHandler trades the signed-in user's token for a one-time WebSocket ticket.
func RealtimeTicketHandler(hub *realtime.Hub) gin.HandlerFunc {
	return func(context *gin.Context) {
		if hub == nil {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error": "live updates are not available"})
			return
		}
		expires, _ := context.Get(tokenExpiryKey)
		tokenExpires, _ := expires.(time.Time)
		context.JSON(http.StatusOK, gin.H{"ticket": hub.IssueTicket(currentUser(context).ID, tokenExpires)})
	}
}

// RealtimeHandler upgrades to a WebSocket. It sits outside the protected group because browsers
// can't send Authorization here: the ticket proves who is connecting, and websocket.Accept refuses
// other origins. Middleware only runs once on a connection this long, so the hub re-checks the
// account as ActiveUser would.
func RealtimeHandler(hub *realtime.Hub, repo *auth.Repository) gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		if hub == nil {
			ginContext.JSON(http.StatusServiceUnavailable, gin.H{"error": "live updates are not available"})
			return
		}
		ticket, ok := hub.RedeemTicket(ginContext.Query("ticket"))
		if !ok {
			ginContext.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired ticket"})
			return
		}
		conn, err := websocket.Accept(ginContext.Writer, ginContext.Request, nil)
		if err != nil {
			log.Printf("realtime: could not upgrade: %v", err) // Accept has written the response
			return
		}
		hub.Serve(ginContext.Request.Context(), conn, realtime.Session{
			UserID:  ticket.UserID,
			Expires: ticket.TokenExpires,
			Check: func(ctx context.Context) error {
				user, err := repo.FindByID(ctx, ticket.UserID)
				if errors.Is(err, sql.ErrNoRows) {
					return realtime.ErrAccessRevoked
				}
				if err != nil {
					return err
				}
				if !user.IsActive {
					return realtime.ErrAccessRevoked
				}
				return nil
			},
		})
	}
}
