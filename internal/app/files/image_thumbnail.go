package files

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"

	"telesrv/internal/domain"

	xdraw "golang.org/x/image/draw"
)

// prepareUploadedImage bounds allocation before decoding untrusted wallpaper
// images. The thumbnail is a real JPEG object available through upload.getFile.
func (s *Service) prepareUploadedImage(ctx context.Context, body assembledUploadBlob, spec domain.DocumentSpec) (domain.DocumentSpec, []byte, error) {
	if body.Size <= 0 || body.Size > avatarSourceMaxBytes {
		return spec, nil, domain.ErrDocumentInvalid
	}
	data, total, err := s.blobs.GetRange(ctx, body.ObjectKey, 0, body.Size)
	if err != nil {
		return spec, nil, err
	}
	if total != body.Size || int64(len(data)) != body.Size || !avatarSourceDecodable(data) {
		return spec, nil, domain.ErrDocumentInvalid
	}
	src, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return spec, nil, domain.ErrDocumentInvalid
	}
	switch format {
	case "jpeg":
		spec.MimeType = "image/jpeg"
	case "png":
		spec.MimeType = "image/png"
	case "webp":
		spec.MimeType = "image/webp"
	default:
		return spec, nil, domain.ErrDocumentInvalid
	}
	bounds := src.Bounds()
	attrs := make([]domain.DocumentAttribute, 0, len(spec.Attributes)+1)
	for _, attr := range spec.Attributes {
		if attr.Kind != domain.DocAttrImageSize {
			attrs = append(attrs, attr)
		}
	}
	spec.Attributes = append(attrs, domain.DocumentAttribute{Kind: domain.DocAttrImageSize, W: bounds.Dx(), H: bounds.Dy()})
	w, h := scaleDown(bounds.Dx(), bounds.Dy(), 320)
	w, h = maxInt(1, w), maxInt(1, h)
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, bounds, xdraw.Src, nil)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: 85}); err != nil {
		return spec, nil, err
	}
	return spec, out.Bytes(), nil
}
