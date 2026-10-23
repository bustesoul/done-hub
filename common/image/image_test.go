package image_test

import (
	"bytes"
	"encoding/base64"
	"image"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"done-hub/common/config"
	img "done-hub/common/image"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var cases = []struct {
	file, format  string
	width, height int
}{
	{"sample.jpeg", "jpeg", 9, 6},
	{"sample.png", "png", 7, 5},
	{"sample.webp", "webp", 1080, 720},
	{"sample.gif", "gif", 5, 4},
	{"small.jpeg", "jpeg", 270, 230},
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	require.NoError(t, err)
	return data
}

func imageServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	for _, c := range cases {
		data := fixture(t, c.file)
		mux.HandleFunc("/"+c.file, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "image/"+c.format)
			_, _ = w.Write(data)
		})
	}
	mux.HandleFunc("/text", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "not an image")
	})
	server := httptest.NewServer(mux)
	oldClient, oldWorker := img.ImageHttpClients, config.CFWorkerImageUrl
	img.ImageHttpClients, config.CFWorkerImageUrl = server.Client(), ""
	t.Cleanup(func() {
		server.Close()
		img.ImageHttpClients, config.CFWorkerImageUrl = oldClient, oldWorker
	})
	return server
}

func TestDecode(t *testing.T) {
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			data := fixture(t, c.file)
			decoded, format, err := image.Decode(bytes.NewReader(data))
			require.NoError(t, err)
			assert.Equal(t, c.format, format)
			assert.Equal(t, image.Pt(c.width, c.height), decoded.Bounds().Size())
			dimensions, format, err := image.DecodeConfig(bytes.NewReader(data))
			require.NoError(t, err)
			assert.Equal(t, c.format, format)
			assert.Equal(t, c.width, dimensions.Width)
			assert.Equal(t, c.height, dimensions.Height)
		})
	}
}

func TestBase64(t *testing.T) {
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			encoded := base64.StdEncoding.EncodeToString(fixture(t, c.file))
			reader := func() io.Reader { return base64.NewDecoder(base64.StdEncoding, strings.NewReader(encoded)) }
			decoded, format, err := image.Decode(reader())
			require.NoError(t, err)
			assert.Equal(t, c.format, format)
			assert.Equal(t, image.Pt(c.width, c.height), decoded.Bounds().Size())
			dimensions, format, err := image.DecodeConfig(reader())
			require.NoError(t, err)
			assert.Equal(t, c.format, format)
			assert.Equal(t, c.width, dimensions.Width)
			assert.Equal(t, c.height, dimensions.Height)
		})
	}
}

func TestGetImageSize(t *testing.T) {
	server := imageServer(t)
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			width, height, err := img.GetImageSize(server.URL + "/" + c.file)
			require.NoError(t, err)
			assert.Equal(t, c.width, width)
			assert.Equal(t, c.height, height)
		})
	}
	_, _, err := img.GetImageSize(server.URL + "/text")
	assert.Error(t, err)
}

func TestGetImageSizeFromBase64(t *testing.T) {
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			encoded := base64.StdEncoding.EncodeToString(fixture(t, c.file))
			for _, input := range []string{encoded, "data:image/" + c.format + ";base64," + encoded} {
				width, height, err := img.GetImageSizeFromBase64(input)
				require.NoError(t, err)
				assert.Equal(t, c.width, width)
				assert.Equal(t, c.height, height)
			}
		})
	}
}

func TestGetImageFromUrl(t *testing.T) {
	server := imageServer(t)
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			encoded := base64.StdEncoding.EncodeToString(fixture(t, c.file))
			for _, input := range []string{server.URL + "/" + c.file, "data:image/" + c.format + ";base64," + encoded} {
				mimeType, data, err := img.GetImageFromUrl(input)
				require.NoError(t, err)
				assert.Equal(t, encoded, data)
				assert.Equal(t, "image/"+c.format, mimeType)
			}
		})
	}
	// This helper returns the content and MIME type; it does not decode images.
	mimeType, data, err := img.GetImageFromUrl(server.URL + "/text")
	require.NoError(t, err)
	assert.Equal(t, "text/plain", mimeType)
	assert.Equal(t, base64.StdEncoding.EncodeToString([]byte("not an image")), data)
	for _, input := range []string{"invalid-url", "data:image/text;base64,"} {
		_, _, err := img.GetImageFromUrl(input)
		assert.Error(t, err)
	}
}

func TestParseBase64File(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString(fixture(t, "sample.png"))
	mimeType, data, err := img.ParseBase64File("data:audio/mpeg;base64," + encoded)
	require.NoError(t, err)
	assert.Equal(t, "audio/mpeg", mimeType)
	assert.Equal(t, encoded, data)
}
