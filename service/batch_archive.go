package service

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/basketikun/infinite-canvas/model"
	"github.com/basketikun/infinite-canvas/repository"
	"gorm.io/gorm"
)

const maxBatchProductionArchiveBytes int64 = 2 << 30

type BatchProductionArchive struct {
	File *os.File
	Name string
	Size int64
}

func (archive BatchProductionArchive) Cleanup() {
	if archive.File == nil { return }
	name := archive.File.Name()
	_ = archive.File.Close()
	_ = os.Remove(name)
}

func CreateBatchProductionArchive(ctx context.Context, user model.AuthUser, jobID string) (BatchProductionArchive, error) {
	organization, _, err := EnsureOrganization(user)
	if err != nil { return BatchProductionArchive{}, err }
	jobID = strings.TrimSpace(jobID)
	job, items, err := repository.ListBatchProductionArchiveItems(organization.ID, jobID)
	if errors.Is(err, gorm.ErrRecordNotFound) { return BatchProductionArchive{}, safeMessageError{message: "批量任务不存在"} }
	if err != nil { return BatchProductionArchive{}, err }
	if len(items) == 0 { return BatchProductionArchive{}, safeMessageError{message: "当前任务没有审核通过的结果"} }
	var totalSize int64
	for _, item := range items {
		if item.Size <= 0 || item.Size > maxUserFileSize || totalSize > maxBatchProductionArchiveBytes-item.Size { return BatchProductionArchive{}, safeMessageError{message: "审核通过的结果总量过大，请分批下载"} }
		totalSize += item.Size
	}
	if err := ensureQiniuStorageConfigured(); err != nil { return BatchProductionArchive{}, err }
	file, err := os.CreateTemp("", "infinite-canvas-batch-*.zip")
	if err != nil { return BatchProductionArchive{}, err }
	archive := BatchProductionArchive{File: file, Name: batchProductionArchiveName(job), Size: totalSize}
	if err := writeBatchProductionArchive(ctx, organization.ID, file, job.DeliverySpec, items); err != nil { archive.Cleanup(); return BatchProductionArchive{}, err }
	info, err := file.Stat()
	if err != nil { archive.Cleanup(); return BatchProductionArchive{}, err }
	archive.Size = info.Size()
	if _, err := file.Seek(0, io.SeekStart); err != nil { archive.Cleanup(); return BatchProductionArchive{}, err }
	return archive, nil
}

func writeBatchProductionArchive(ctx context.Context, organizationID string, target io.Writer, spec model.ProductionDeliverySpec, items []model.BatchProductionArchiveItem) error {
	writer := zip.NewWriter(target)
	client := &http.Client{
		Timeout: 10 * time.Minute,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 5 || !validHTTPSURL(request.URL.String()) { return errors.New("batch archive redirect is not allowed") }
			return nil
		},
	}
	for _, item := range items {
		value, ok := organizationFileURL(organizationID, item.ResultStorageKey, 30*time.Minute)
		if !ok { _ = writer.Close(); return errors.New("batch archive source is unavailable") }
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, value, nil)
		if err != nil { _ = writer.Close(); return err }
		response, err := client.Do(request)
		if err != nil { _ = writer.Close(); return err }
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			response.Body.Close()
			_ = writer.Close()
			return fmt.Errorf("batch archive source returned HTTP %d", response.StatusCode)
		}
		entry, err := writer.CreateHeader(&zip.FileHeader{Name: batchProductionArchiveEntryName(item, spec), Method: zip.Store})
		if err != nil { response.Body.Close(); _ = writer.Close(); return err }
		written, copyErr := io.CopyN(entry, response.Body, item.Size+1)
		response.Body.Close()
		if written != item.Size || (copyErr != nil && !errors.Is(copyErr, io.EOF)) { _ = writer.Close(); return errors.New("batch archive source size changed") }
	}
	return writer.Close()
}

func batchProductionArchiveName(job model.BatchProductionJob) string {
	name := safeBatchArchiveSegment(job.Name)
	if name == "" { name = safeBatchArchiveSegment(job.ID) }
	if name == "" { name = "batch-results" }
	return name + ".zip"
}

func batchProductionArchiveEntryName(item model.BatchProductionArchiveItem, spec model.ProductionDeliverySpec) string {
	product := safeBatchArchiveSegment(item.ProductCode)
	if product == "" { product = safeBatchArchiveSegment(item.ProductID) }
	sku := safeBatchArchiveSegment(item.SKUCode)
	if sku == "" && item.SKUID != "" { sku = safeBatchArchiveSegment(item.SKUID) }
	if sku == "" { sku = "SPU" }
	id := safeBatchArchiveSegment(item.ID)
	if runes := []rune(id); len(runes) > 10 { id = string(runes[len(runes)-10:]) }
	role := "结果"
	if item.IsPrimary { role = "主图" }
	pattern := spec.FilenamePattern
	if strings.TrimSpace(pattern) == "" { pattern = "{spu}_{sku}_{role}_{item}" }
	for key, value := range map[string]string{"{spu}": product, "{sku}": sku, "{role}": role, "{item}": id} { pattern = strings.ReplaceAll(pattern, key, value) }
	name := safeBatchArchiveSegment(pattern)
	if name == "" { name = sku + "_" + role + "_" + id }
	return product + "/" + name + batchArchiveExtension(item.MimeType)
}

func safeBatchArchiveSegment(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Map(func(char rune) rune {
		if unicode.IsLetter(char) || unicode.IsNumber(char) || char == '-' || char == '_' { return char }
		if unicode.IsSpace(char) { return '-' }
		return -1
	}, value)
	for strings.Contains(value, "--") { value = strings.ReplaceAll(value, "--", "-") }
	value = strings.Trim(value, "-")
	runes := []rune(value)
	if len(runes) > 80 { value = string(runes[:80]) }
	return value
}

func batchArchiveExtension(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(strings.Split(mimeType, ";")[0])) {
	case "image/jpeg": return ".jpg"
	case "image/png": return ".png"
	case "image/webp": return ".webp"
	case "image/gif": return ".gif"
	case "image/avif": return ".avif"
	default: return ".bin"
	}
}
