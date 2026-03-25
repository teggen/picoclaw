package agent

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sipeed/picoclaw/pkg/media"
	"github.com/sipeed/picoclaw/pkg/providers"
)

// stubMediaStore is a minimal in-memory MediaStore for testing media resolution.
type stubMediaStore struct {
	entries map[string]struct {
		path string
		meta media.MediaMeta
	}
}

func newStubMediaStore() *stubMediaStore {
	return &stubMediaStore{
		entries: make(map[string]struct {
			path string
			meta media.MediaMeta
		}),
	}
}

func (s *stubMediaStore) Store(localPath string, meta media.MediaMeta, scope string) (string, error) {
	ref := "media://" + filepath.Base(localPath)
	s.entries[ref] = struct {
		path string
		meta media.MediaMeta
	}{localPath, meta}
	return ref, nil
}

func (s *stubMediaStore) Resolve(ref string) (string, error) {
	e, ok := s.entries[ref]
	if !ok {
		return "", os.ErrNotExist
	}
	return e.path, nil
}

func (s *stubMediaStore) ResolveWithMeta(ref string) (string, media.MediaMeta, error) {
	e, ok := s.entries[ref]
	if !ok {
		return "", media.MediaMeta{}, os.ErrNotExist
	}
	return e.path, e.meta, nil
}

func (s *stubMediaStore) ReleaseAll(scope string) error { return nil }

// --- buildPathTag tests ---

func TestBuildPathTag_Audio(t *testing.T) {
	got := buildPathTag("audio/mpeg", "/tmp/song.mp3")
	want := "[audio:/tmp/song.mp3]"
	if got != want {
		t.Errorf("buildPathTag(audio) = %q, want %q", got, want)
	}
}

func TestBuildPathTag_Video(t *testing.T) {
	got := buildPathTag("video/mp4", "/tmp/clip.mp4")
	want := "[video:/tmp/clip.mp4]"
	if got != want {
		t.Errorf("buildPathTag(video) = %q, want %q", got, want)
	}
}

func TestBuildPathTag_Default(t *testing.T) {
	got := buildPathTag("application/pdf", "/tmp/doc.pdf")
	want := "[file:/tmp/doc.pdf]"
	if got != want {
		t.Errorf("buildPathTag(default) = %q, want %q", got, want)
	}
}

func TestBuildPathTag_EmptyMIME(t *testing.T) {
	got := buildPathTag("", "/tmp/unknown")
	want := "[file:/tmp/unknown]"
	if got != want {
		t.Errorf("buildPathTag(empty) = %q, want %q", got, want)
	}
}

// --- injectPathTags tests ---

func TestInjectPathTags_ReplacesGenericAudio(t *testing.T) {
	content := "Here is the audio: [audio]"
	tags := []string{"[audio:/tmp/song.mp3]"}
	got := injectPathTags(content, tags)
	want := "Here is the audio: [audio:/tmp/song.mp3]"
	if got != want {
		t.Errorf("injectPathTags = %q, want %q", got, want)
	}
}

func TestInjectPathTags_ReplacesGenericVideo(t *testing.T) {
	content := "Watch: [video]"
	tags := []string{"[video:/tmp/clip.mp4]"}
	got := injectPathTags(content, tags)
	want := "Watch: [video:/tmp/clip.mp4]"
	if got != want {
		t.Errorf("injectPathTags = %q, want %q", got, want)
	}
}

func TestInjectPathTags_ReplacesGenericFile(t *testing.T) {
	content := "See [file]"
	tags := []string{"[file:/tmp/doc.pdf]"}
	got := injectPathTags(content, tags)
	want := "See [file:/tmp/doc.pdf]"
	if got != want {
		t.Errorf("injectPathTags = %q, want %q", got, want)
	}
}

func TestInjectPathTags_AppendsWhenNoGenericTag(t *testing.T) {
	content := "Some text"
	tags := []string{"[file:/tmp/doc.pdf]"}
	got := injectPathTags(content, tags)
	want := "Some text [file:/tmp/doc.pdf]"
	if got != want {
		t.Errorf("injectPathTags = %q, want %q", got, want)
	}
}

func TestInjectPathTags_EmptyContent(t *testing.T) {
	tags := []string{"[file:/tmp/doc.pdf]"}
	got := injectPathTags("", tags)
	want := "[file:/tmp/doc.pdf]"
	if got != want {
		t.Errorf("injectPathTags(empty) = %q, want %q", got, want)
	}
}

func TestInjectPathTags_MultipleTags(t *testing.T) {
	content := "[audio] and [file]"
	tags := []string{"[audio:/tmp/a.mp3]", "[file:/tmp/b.pdf]"}
	got := injectPathTags(content, tags)
	if !strings.Contains(got, "[audio:/tmp/a.mp3]") {
		t.Errorf("expected audio tag in %q", got)
	}
	if !strings.Contains(got, "[file:/tmp/b.pdf]") {
		t.Errorf("expected file tag in %q", got)
	}
}

// --- detectMIME tests ---

func TestDetectMIME_FromMeta(t *testing.T) {
	meta := media.MediaMeta{ContentType: "application/pdf"}
	got := detectMIME("/nonexistent", meta)
	if got != "application/pdf" {
		t.Errorf("detectMIME(meta) = %q, want %q", got, "application/pdf")
	}
}

func TestDetectMIME_EmptyMeta_NonexistentFile(t *testing.T) {
	got := detectMIME("/nonexistent/file.xyz", media.MediaMeta{})
	if got != "" {
		t.Errorf("detectMIME(bad file) = %q, want empty", got)
	}
}

func TestDetectMIME_RealPNG(t *testing.T) {
	// Write a minimal PNG file (8-byte header is enough for filetype detection).
	tmp := t.TempDir()
	pngPath := filepath.Join(tmp, "test.png")
	// Minimal PNG header bytes
	pngHeader := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
	}
	os.WriteFile(pngPath, pngHeader, 0o644)

	got := detectMIME(pngPath, media.MediaMeta{})
	if got != "image/png" {
		t.Errorf("detectMIME(png) = %q, want image/png", got)
	}
}

// --- encodeImageToDataURL tests ---

func TestEncodeImageToDataURL_SmallFile(t *testing.T) {
	tmp := t.TempDir()
	imgPath := filepath.Join(tmp, "tiny.png")
	data := []byte("fake-image-data")
	os.WriteFile(imgPath, data, 0o644)
	info, _ := os.Stat(imgPath)

	result := encodeImageToDataURL(imgPath, "image/png", info, 1024)
	if !strings.HasPrefix(result, "data:image/png;base64,") {
		t.Errorf("expected data URL prefix, got %q", result[:40])
	}
	// Verify the base64 content decodes to the original data.
	encoded := strings.TrimPrefix(result, "data:image/png;base64,")
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("base64 decode failed: %v", err)
	}
	if string(decoded) != string(data) {
		t.Errorf("decoded data mismatch")
	}
}

func TestEncodeImageToDataURL_TooLarge(t *testing.T) {
	tmp := t.TempDir()
	imgPath := filepath.Join(tmp, "big.png")
	data := make([]byte, 100)
	os.WriteFile(imgPath, data, 0o644)
	info, _ := os.Stat(imgPath)

	result := encodeImageToDataURL(imgPath, "image/png", info, 50)
	if result != "" {
		t.Errorf("expected empty for oversized file, got %q", result)
	}
}

func TestEncodeImageToDataURL_NonexistentFile(t *testing.T) {
	// Create a fake FileInfo by writing then deleting.
	tmp := t.TempDir()
	imgPath := filepath.Join(tmp, "gone.png")
	os.WriteFile(imgPath, []byte("x"), 0o644)
	info, _ := os.Stat(imgPath)
	os.Remove(imgPath)

	result := encodeImageToDataURL(imgPath, "image/png", info, 1024)
	if result != "" {
		t.Errorf("expected empty for missing file, got %q", result)
	}
}

// --- resolveMediaRefs tests ---

func TestResolveMediaRefs_NilStore(t *testing.T) {
	msgs := []providers.Message{{Content: "hello", Media: []string{"media://x"}}}
	got := resolveMediaRefs(msgs, nil, 1024)
	if len(got) != 1 || got[0].Content != "hello" {
		t.Errorf("nil store should return messages unchanged")
	}
}

func TestResolveMediaRefs_NoMedia(t *testing.T) {
	store := newStubMediaStore()
	msgs := []providers.Message{{Content: "hello"}}
	got := resolveMediaRefs(msgs, store, 1024)
	if len(got) != 1 || got[0].Content != "hello" {
		t.Errorf("no media should return messages unchanged")
	}
}

func TestResolveMediaRefs_NonMediaRef(t *testing.T) {
	store := newStubMediaStore()
	msgs := []providers.Message{{Content: "hello", Media: []string{"data:image/png;base64,abc"}}}
	got := resolveMediaRefs(msgs, store, 1024)
	if len(got[0].Media) != 1 || got[0].Media[0] != "data:image/png;base64,abc" {
		t.Errorf("non-media:// refs should be preserved")
	}
}

func TestResolveMediaRefs_ImageRef(t *testing.T) {
	tmp := t.TempDir()
	imgPath := filepath.Join(tmp, "photo.png")
	os.WriteFile(imgPath, []byte("img-data"), 0o644)

	store := newStubMediaStore()
	store.entries["media://photo.png"] = struct {
		path string
		meta media.MediaMeta
	}{imgPath, media.MediaMeta{ContentType: "image/jpeg"}}

	msgs := []providers.Message{{Content: "look", Media: []string{"media://photo.png"}}}
	got := resolveMediaRefs(msgs, store, 1024)

	if len(got[0].Media) != 1 {
		t.Fatalf("expected 1 resolved media, got %d", len(got[0].Media))
	}
	if !strings.HasPrefix(got[0].Media[0], "data:image/jpeg;base64,") {
		t.Errorf("expected base64 data URL, got %q", got[0].Media[0])
	}
}

func TestResolveMediaRefs_NonImageRef_InjectsPathTag(t *testing.T) {
	tmp := t.TempDir()
	pdfPath := filepath.Join(tmp, "doc.pdf")
	os.WriteFile(pdfPath, []byte("pdf-data"), 0o644)

	store := newStubMediaStore()
	store.entries["media://doc.pdf"] = struct {
		path string
		meta media.MediaMeta
	}{pdfPath, media.MediaMeta{ContentType: "application/pdf"}}

	msgs := []providers.Message{{Content: "check this", Media: []string{"media://doc.pdf"}}}
	got := resolveMediaRefs(msgs, store, 1024)

	// Non-image should not be in Media array.
	if len(got[0].Media) != 0 {
		t.Errorf("non-image should not be in Media, got %v", got[0].Media)
	}
	// Path tag should be injected into Content.
	if !strings.Contains(got[0].Content, "[file:"+pdfPath+"]") {
		t.Errorf("expected path tag in content, got %q", got[0].Content)
	}
}

func TestResolveMediaRefs_PreservesOriginalMessages(t *testing.T) {
	tmp := t.TempDir()
	pdfPath := filepath.Join(tmp, "doc.pdf")
	os.WriteFile(pdfPath, []byte("pdf-data"), 0o644)

	store := newStubMediaStore()
	store.entries["media://doc.pdf"] = struct {
		path string
		meta media.MediaMeta
	}{pdfPath, media.MediaMeta{ContentType: "application/pdf"}}

	original := []providers.Message{{Content: "original", Media: []string{"media://doc.pdf"}}}
	_ = resolveMediaRefs(original, store, 1024)

	if original[0].Content != "original" {
		t.Errorf("original message content was mutated to %q", original[0].Content)
	}
}

func TestResolveMediaRefs_UnresolvableRef(t *testing.T) {
	store := newStubMediaStore()
	msgs := []providers.Message{{Content: "text", Media: []string{"media://missing"}}}
	got := resolveMediaRefs(msgs, store, 1024)

	// Unresolvable ref should be dropped.
	if len(got[0].Media) != 0 {
		t.Errorf("unresolvable ref should be dropped, got %v", got[0].Media)
	}
}

// --- buildArtifactTags tests ---

func TestBuildArtifactTags_NilStore(t *testing.T) {
	got := buildArtifactTags(nil, []string{"media://x"})
	if got != nil {
		t.Errorf("expected nil for nil store, got %v", got)
	}
}

func TestBuildArtifactTags_EmptyRefs(t *testing.T) {
	store := newStubMediaStore()
	got := buildArtifactTags(store, nil)
	if got != nil {
		t.Errorf("expected nil for empty refs, got %v", got)
	}
}

func TestBuildArtifactTags_ValidRefs(t *testing.T) {
	tmp := t.TempDir()
	audioPath := filepath.Join(tmp, "song.mp3")
	os.WriteFile(audioPath, []byte("audio"), 0o644)

	store := newStubMediaStore()
	store.entries["media://song.mp3"] = struct {
		path string
		meta media.MediaMeta
	}{audioPath, media.MediaMeta{ContentType: "audio/mpeg"}}

	got := buildArtifactTags(store, []string{"media://song.mp3"})
	if len(got) != 1 {
		t.Fatalf("expected 1 tag, got %d", len(got))
	}
	if got[0] != "[audio:"+audioPath+"]" {
		t.Errorf("expected audio tag, got %q", got[0])
	}
}

func TestBuildArtifactTags_SkipsUnresolvable(t *testing.T) {
	store := newStubMediaStore()
	got := buildArtifactTags(store, []string{"media://missing"})
	if len(got) != 0 {
		t.Errorf("expected empty for unresolvable refs, got %v", got)
	}
}
