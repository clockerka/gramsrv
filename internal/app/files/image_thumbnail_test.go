package files

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"testing"

	"telesrv/internal/domain"
)

func TestUploadedWallpaperHasRealDimensionsAndDownloadableThumbnail(t *testing.T) {
	ctx := context.Background()
	media := newFakeMediaStore()
	svc, _ := newUploadPartTestService(t, media, domain.UploadPartQuota{})
	data := testJPEG(t, 480, 800)
	file := domain.UploadedFileRef{OwnerUserID: 10, FileID: 9631, Parts: 1, Name: "portrait.jpg"}
	if _, err := svc.SaveFilePart(ctx, file.OwnerUserID, file.FileID, 0, data); err != nil {
		t.Fatal(err)
	}
	spec := domain.DocumentSpec{MimeType: "image/jpeg", ForceFile: true, GenerateImageThumbnail: true,
		Attributes: []domain.DocumentAttribute{{Kind: domain.DocAttrFilename, FileName: file.Name}}}
	doc, err := svc.CreateDocumentFromUpload(ctx, file, spec)
	if err != nil {
		t.Fatal(err)
	}
	var w, h int
	for _, attr := range doc.Attributes {
		if attr.Kind == domain.DocAttrImageSize {
			w, h = attr.W, attr.H
		}
	}
	if w != 480 || h != 800 {
		t.Fatalf("image size=%dx%d, want 480x800", w, h)
	}
	if len(doc.Thumbs) != 1 {
		t.Fatalf("thumbnail count=%d", len(doc.Thumbs))
	}
	thumb := doc.Thumbs[0]
	chunk, found, err := svc.GetFile(ctx, domain.FileDownloadRequest{LocationKey: fmt.Sprintf("doc:%d:%s", doc.ID, thumb.Type), Limit: 1 << 20})
	if err != nil || !found {
		t.Fatalf("thumbnail download: found=%v err=%v", found, err)
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(chunk.Bytes))
	if err != nil || format != "jpeg" || cfg.Width != 192 || cfg.Height != 320 || thumb.W != cfg.Width || thumb.H != cfg.Height {
		t.Fatalf("thumbnail=%+v format=%s size=%+v err=%v", thumb, format, cfg, err)
	}
	replay, err := svc.CreateDocumentFromUpload(ctx, file, spec)
	if err != nil || replay.ID != doc.ID || len(replay.Thumbs) != 1 {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}
	body, found, err := svc.GetFile(ctx, domain.FileDownloadRequest{LocationKey: fmt.Sprintf("doc:%d", doc.ID), Limit: 1 << 20})
	if err != nil || !found || !bytes.Equal(body.Bytes, data) {
		t.Fatalf("wallpaper bytes changed: found=%v err=%v", found, err)
	}
}

func TestUploadedWallpaperRejectsInvalidAndOversizedImages(t *testing.T) {
	for i, data := range [][]byte{[]byte("not an image"), testPNGHeader(t, 6000, 5000)} {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			media := newFakeMediaStore()
			svc, _ := newUploadPartTestService(t, media, domain.UploadPartQuota{})
			file := domain.UploadedFileRef{OwnerUserID: 10, FileID: 9632, Parts: 1}
			if _, err := svc.SaveFilePart(context.Background(), file.OwnerUserID, file.FileID, 0, data); err != nil {
				t.Fatal(err)
			}
			_, err := svc.CreateDocumentFromUpload(context.Background(), file, domain.DocumentSpec{MimeType: "image/jpeg", ForceFile: true, GenerateImageThumbnail: true})
			if !errors.Is(err, domain.ErrDocumentInvalid) {
				t.Fatalf("err=%v, want ErrDocumentInvalid", err)
			}
		})
	}
}
