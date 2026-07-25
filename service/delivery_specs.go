package service

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"net/http"
	"strings"
	"time"

	"github.com/basketikun/infinite-canvas/model"
	"github.com/disintegration/imaging"
)

var productionDeliverySpecs = []model.ProductionDeliverySpec{
	{ID: "original", Platform: "通用", Name: "保留原始结果", Format: "original", FilenamePattern: "{spu}_{sku}_{role}_{item}"},
	{ID: "taobao-main", Platform: "淘宝", Name: "商品主图", Width: 800, Height: 800, Format: "jpeg", Quality: 92, FilenamePattern: "{spu}_{sku}_{role}"},
	{ID: "jd-main", Platform: "京东", Name: "商品主图", Width: 800, Height: 800, Format: "jpeg", Quality: 92, FilenamePattern: "{spu}_{sku}_{role}"},
	{ID: "douyin-product", Platform: "抖音", Name: "商品竖图", Width: 1080, Height: 1440, Format: "jpeg", Quality: 90, FilenamePattern: "{spu}_{sku}_{role}"},
	{ID: "xiaohongshu-cover", Platform: "小红书", Name: "商品封面", Width: 1242, Height: 1660, Format: "jpeg", Quality: 90, FilenamePattern: "{spu}_{sku}_{role}"},
}

func ListProductionDeliverySpecs() []model.ProductionDeliverySpec {
	items := make([]model.ProductionDeliverySpec, len(productionDeliverySpecs))
	copy(items, productionDeliverySpecs)
	return items
}

func resolveProductionDeliverySpec(id string) (model.ProductionDeliverySpec, error) {
	id = strings.TrimSpace(id)
	if id == "" { id = "original" }
	for _, item := range productionDeliverySpecs {
		if item.ID == id { return item, nil }
	}
	return model.ProductionDeliverySpec{}, safeMessageError{message: "渠道交付规格无效"}
}

func productionDeliveryGenerationSize(spec model.ProductionDeliverySpec) string {
	if spec.Width > spec.Height { return "1536x1024" }
	if spec.Height > spec.Width { return "1024x1536" }
	return standardBatchImageSize
}

func prepareProductionDeliveryResult(ctx context.Context, result BatchProductionResult, spec model.ProductionDeliverySpec) (BatchProductionResult, error) {
	if spec.Width <= 0 || spec.Height <= 0 || spec.Format == "original" { return result, nil }
	if len(result.Data) == 0 {
		transport := batchResultTransport()
		defer transport.CloseIdleConnections()
		client := &http.Client{Transport: transport, Timeout: 15 * time.Minute, CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 5 || !validHTTPSURL(request.URL.String()) { return errors.New("batch delivery redirect is not allowed") }
			return nil
		}}
		data, mimeType, err := downloadStandardBatchImage(ctx, client, result.ResultURL, maxUserFileSize)
		if err != nil { return result, err }
		if int64(len(data)) != result.Size { return result, errors.New("batch delivery source size does not match executor response") }
		if declared := strings.TrimSpace(strings.Split(result.MimeType, ";")[0]); declared != "" && declared != mimeType { return result, errors.New("batch delivery source MIME does not match executor response") }
		result.Data, result.MimeType, result.Size = data, mimeType, int64(len(data))
	} else if int64(len(result.Data)) != result.Size {
		return result, errors.New("batch delivery source size does not match executor response")
	}
	return applyProductionDeliverySpec(result, spec)
}

func applyProductionDeliverySpec(result BatchProductionResult, spec model.ProductionDeliverySpec) (BatchProductionResult, error) {
	if spec.Width <= 0 || spec.Height <= 0 || spec.Format == "original" { return result, nil }
	source, err := imaging.Decode(bytes.NewReader(result.Data))
	if err != nil { return result, errors.New("generated image cannot be decoded for delivery") }
	imageResult := imaging.Fill(source, spec.Width, spec.Height, imaging.Center, imaging.Lanczos)
	var output bytes.Buffer
	switch spec.Format {
	case "jpeg":
		background := imaging.New(spec.Width, spec.Height, color.White)
		flattened := imaging.Overlay(background, imageResult, image.Point{}, 1)
		if err := imaging.Encode(&output, flattened, imaging.JPEG, imaging.JPEGQuality(spec.Quality)); err != nil { return result, err }
		result.MimeType = "image/jpeg"
	case "png":
		if err := imaging.Encode(&output, imageResult, imaging.PNG); err != nil { return result, err }
		result.MimeType = "image/png"
	default:
		return result, errors.New("delivery image format is invalid")
	}
	if output.Len() == 0 || output.Len() > maxUserFileSize { return result, errors.New("delivery image is too large") }
	result.Data, result.Size, result.ResultURL = output.Bytes(), int64(output.Len()), ""
	return result, nil
}
