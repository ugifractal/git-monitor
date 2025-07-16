package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	"github.com/uptrace/bun/extra/bundebug"
)

type GithubEvent struct {
	bun.BaseModel `bun:"table:github_events"`

	ID          int64          `bun:",pk,autoincrement"`
	PusherName  string         `bun:",notnull"`
	PusherEmail string         `bun:",notnull"`
	Payload     map[string]any `bun:"type:jsonb,notnull,default:'{}'"`
	CreatedAt   time.Time      `bun:"created_at,nullzero,notnull,default:now()"`
	UpdatedAt   time.Time      `bun:"updated_at,nullzero,notnull,default:now()"`
	CommitAt    time.Time      `bun:"commit_at,nullzero,notnull,default:now()"`
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Println("Error loading .env file")
	}
	dsn := os.Getenv("DATABASE_URL")
	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))

	db := bun.NewDB(sqldb, pgdialect.New())
	db.AddQueryHook(bundebug.NewQueryHook(
		bundebug.WithVerbose(true),
		bundebug.FromEnv("BUNDEBUG"),
	))

	r := gin.Default()
	r.GET("/ping", pingHandler)
	r.POST("/github", verifyGitHubSignature, githubWebhookHandler(*db))
	r.GET("/last_push", lastPushHandler(*db))
	r.Run("0.0.0.0:3000")
}

func pingHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"message": "pong",
	})
}

func lastPushHandler(db bun.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		githubEvent := new(GithubEvent)
		name := os.Getenv("MONITORED_USERNAME")
		err := db.NewSelect().Model(githubEvent).Where("pusher_name = ?", name).Order("commit_at DESC").Scan(ctx)
		if err != nil {
			panic(err)
		}

		old := time.Now().UTC().Sub(githubEvent.CommitAt)

		c.JSON(http.StatusOK, gin.H{
			"pusher_name":  githubEvent.PusherName,
			"pusher_email": githubEvent.PusherEmail,
			"commit_at":    githubEvent.CommitAt.Format(time.RFC3339),
			"old":          old.Hours(),
		})

	}
}

func githubWebhookHandler(db bun.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		var data map[string]interface{}

		if err := c.BindJSON(&data); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}

		pusher, ok := data["pusher"].(map[string]interface{})
		if !ok {
			return
		}
		var pusherEmail = pusher["email"].(string)
		var pusherName = pusher["name"].(string)

		head_commit, ok := data["head_commit"].(map[string]interface{})
		if !ok {
			return
		}
		var timestamp = head_commit["timestamp"].(string)
		t, _ := time.Parse(time.RFC3339, timestamp)
		utc := t.UTC()

		github_event := &GithubEvent{
			PusherName:  pusherName,
			PusherEmail: pusherEmail,
			Payload:     data,
			CommitAt:    utc,
		}

		res, err := db.NewInsert().Model(github_event).Exec(ctx)

		fmt.Println(res)
		fmt.Println(err)

		c.JSON(http.StatusOK, gin.H{
			"message": "cool",
		})
	}
}

func verifyGitHubSignature(c *gin.Context) {
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to read body"})
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	signature := c.GetHeader("X-Hub-Signature-256")
	if signature == "" {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing signature"})
		return
	}

	var githubSecret = os.Getenv("GITHUB_WEBHOOK_SECRET")

	mac := hmac.New(sha256.New, []byte(githubSecret))
	mac.Write(bodyBytes)
	expectedMAC := mac.Sum(nil)
	expectedSig := "sha256=" + hex.EncodeToString(expectedMAC)

	if !hmac.Equal([]byte(expectedSig), []byte(signature)) {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid signature"})
		return
	}

	c.Next()
}
