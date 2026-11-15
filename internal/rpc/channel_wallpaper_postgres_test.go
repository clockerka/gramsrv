package rpc

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"testing"
	"time"

	"github.com/iamxvbaba/td/clock"
	"github.com/iamxvbaba/td/tg"
	"go.uber.org/zap/zaptest"

	appchannels "telesrv/internal/app/channels"
	appfiles "telesrv/internal/app/files"
	appusers "telesrv/internal/app/users"
	"telesrv/internal/domain"
	"telesrv/internal/store/postgres"
)

func TestChannelUploadedWallpaperPersistsPostgres(t *testing.T) {
	pool := testPremiumPostgresPool(t)
	ctx := context.Background()
	users := postgres.NewUserStore(pool)
	owner, err := users.Create(ctx, domain.User{AccessHash: 9137, Phone: fmt.Sprintf("+1998%d", time.Now().UnixNano()%1_000_000_000), FirstName: "WallpaperOwner"})
	if err != nil {
		t.Fatal(err)
	}
	channelStore := postgres.NewChannelStore(pool)
	created, err := channelStore.CreateChannel(ctx, domain.CreateChannelRequest{CreatorUserID: owner.ID, Title: "Uploaded wallpaper", Broadcast: true, Date: 1700100000})
	if err != nil {
		t.Fatal(err)
	}
	channelID := created.Channel.ID
	var docID int64
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DELETE FROM channel_update_events WHERE channel_id=$1", channelID)
		_, _ = pool.Exec(ctx, "DELETE FROM channel_messages WHERE channel_id=$1", channelID)
		_, _ = pool.Exec(ctx, "DELETE FROM channels WHERE id=$1", channelID)
		_, _ = pool.Exec(ctx, "DELETE FROM uploaded_media_receipts WHERE owner_user_id=$1", owner.ID)
		_, _ = pool.Exec(ctx, "DELETE FROM file_blobs WHERE location_key=$1 OR location_key=$2", fmt.Sprintf("doc:%d", docID), fmt.Sprintf("doc:%d:m", docID))
		_, _ = pool.Exec(ctx, "DELETE FROM documents WHERE id=$1", docID)
		_, _ = pool.Exec(ctx, "DELETE FROM peer_usernames WHERE peer_type='user' AND peer_id=$1", owner.ID)
		_, _ = pool.Exec(ctx, "DELETE FROM users WHERE id=$1", owner.ID)
	})
	blobs, err := appfiles.NewLocalFS(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	newRouter := func() *Router {
		return New(Config{}, Deps{Files: appfiles.NewService(postgres.NewMediaStore(pool), blobs, 2), Users: appusers.NewService(users), Channels: appchannels.NewService(postgres.NewChannelStore(pool))}, zaptest.NewLogger(t), clock.System)
	}
	router := newRouter()
	ctx = WithUserID(ctx, owner.ID)
	var imageBytes bytes.Buffer
	if err := jpeg.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 480, 800)), nil); err != nil {
		t.Fatal(err)
	}
	fileID := time.Now().UnixNano()
	if _, err := router.onUploadSaveFilePart(ctx, &tg.UploadSaveFilePartRequest{FileID: fileID, FilePart: 0, Bytes: imageBytes.Bytes()}); err != nil {
		t.Fatal(err)
	}
	uploaded, err := router.onAccountUploadWallPaper(ctx, &tg.AccountUploadWallPaperRequest{ForChat: true, File: &tg.InputFile{ID: fileID, Parts: 1, Name: "portrait.jpg"}, MimeType: "image/jpeg"})
	if err != nil {
		t.Fatal(err)
	}
	wallpaper, ok := uploaded.(*tg.WallPaper)
	if !ok {
		t.Fatalf("wallpaper=%T", uploaded)
	}
	document, ok := wallpaper.Document.(*tg.Document)
	if !ok || len(document.Thumbs) == 0 {
		t.Fatalf("wallpaper has no document thumbnail: %+v", wallpaper)
	}
	docID = document.ID
	request := &tg.MessagesSetChatWallPaperRequest{Peer: &tg.InputPeerChannel{ChannelID: channelID, AccessHash: created.Channel.AccessHash}}
	request.SetWallpaper(&tg.InputWallPaper{ID: wallpaper.ID, AccessHash: wallpaper.AccessHash})
	request.SetSettings(tg.WallPaperSettings{Blur: true})
	if _, err := router.onMessagesSetChatWallPaper(ctx, request); err != nil {
		t.Fatal(err)
	}
	// A fresh router and stores must reconstruct the same wallpaper from PostgreSQL.
	reloaded := newRouter()
	full, err := reloaded.onChannelsGetFullChannel(ctx, &tg.InputChannel{ChannelID: channelID, AccessHash: created.Channel.AccessHash})
	if err != nil {
		t.Fatal(err)
	}
	channelFull, ok := full.FullChat.(*tg.ChannelFull)
	if !ok {
		t.Fatalf("full chat=%T", full.FullChat)
	}
	stored, ok := channelFull.GetWallpaper()
	if !ok {
		t.Fatal("wallpaper disappeared after reopening channel")
	}
	storedWallpaper, ok := stored.(*tg.WallPaper)
	if !ok || storedWallpaper.ID != wallpaper.ID {
		t.Fatalf("stored wallpaper=%+v", stored)
	}
	storedDoc, ok := storedWallpaper.Document.(*tg.Document)
	if !ok || storedDoc.ID != docID || len(storedDoc.Thumbs) == 0 {
		t.Fatal("wallpaper document/thumbnail disappeared after reload")
	}
	settings, ok := storedWallpaper.GetSettings()
	if !ok || !settings.GetBlur() {
		t.Fatal("wallpaper settings disappeared after reload")
	}
	thumb, ok := storedDoc.Thumbs[0].(*tg.PhotoSize)
	if !ok {
		t.Fatalf("thumbnail=%T", storedDoc.Thumbs[0])
	}
	result, err := reloaded.onUploadGetFile(ctx, &tg.UploadGetFileRequest{Location: &tg.InputDocumentFileLocation{ID: docID, AccessHash: storedDoc.AccessHash, FileReference: storedDoc.FileReference, ThumbSize: thumb.Type}, Limit: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	downloaded, ok := result.(*tg.UploadFile)
	if !ok {
		t.Fatalf("download=%T", result)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(downloaded.Bytes))
	if err != nil || cfg.Width != 192 || cfg.Height != 320 {
		t.Fatalf("thumbnail size=%+v err=%v", cfg, err)
	}
}
