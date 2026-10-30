package contract_test

// loadSpec, toJSON, serveAs (llm_test.go), strict (health_test.go) and visionSpec
// (vision_test.go) are in this package.

import (
	"bytes"
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/VelimirMueller/synthwerk-contracts/go/vision"
)

// multipartImage builds the `image` form body of the classification and upload routes.
func multipartImage(t *testing.T, data []byte) (string, *bytes.Buffer) {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("image", "photo.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return w.FormDataContentType(), &body
}

// widgetBearer sets the widget token and the origin it is bound to.
func widgetBearer(_ context.Context, r *http.Request) error {
	r.Header.Set("Authorization", "Bearer widget-token")
	r.Header.Set("Origin", "https://shop.example")
	return nil
}

// errorCase is one error status and the component response that holds its example.
type errorCase struct {
	status   int
	response string
}

// imageRequest is what the fake vision saw.
type imageRequest struct {
	method, path, auth, origin string
	image                      []byte
}

// fakeVision reads the multipart `image` field and answers with status and body.
func fakeVision(t *testing.T, status int, body []byte, got *imageRequest) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method, got.path = r.Method, r.URL.Path
		got.auth, got.origin = r.Header.Get("Authorization"), r.Header.Get("Origin")
		mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err == nil && mediaType == "multipart/form-data" {
			reader := multipart.NewReader(r.Body, params["boundary"])
			for part, err := reader.NextPart(); err == nil; part, err = reader.NextPart() {
				if part.FormName() == "image" {
					got.image, _ = io.ReadAll(part)
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestVisionWidgetContract(t *testing.T) {
	ctx := context.Background()
	spec := loadSpec(t, visionSpec)
	appJSON := "application/json"
	classify := spec.Paths["/v1/classifications"].Post
	upload := spec.Paths["/v1/images"].Post
	photo := []byte("\xff\xd8\xff\xe0 not really a jpeg")

	t.Run("Classification - spec example - decodes strictly, no correction", func(t *testing.T) {
		var c vision.Classification
		strict(t, toJSON(t, classify.Responses["200"].Content[appJSON].Example), &c)
		if c.Primary.Id != "open:hut" || len(c.Primary.Path) != 3 || c.Topic.Parent != nil {
			t.Fatalf("got %+v", c)
		}
		if c.Correction != nil || c.Uncertain || c.TimingsMs["total"] <= 0 {
			t.Fatalf("correction %+v uncertain %v timings %v", c.Correction, c.Uncertain, c.TimingsMs)
		}
		if c.Topics[1].Parent == nil || *c.Topics[1].Parent != "building and structure" {
			t.Fatalf("child topic: got %+v", c.Topics[1])
		}
	})

	t.Run("Classification - with a correction - decodes the correction strictly", func(t *testing.T) {
		body := []byte(`{"request_id":"r1",
			"image":{"sha256":"ab","content_type":"image/png","size_bytes":10,"width":1,"height":1},
			"model":{"name":"m","taxonomy_version":1},
			"primary":{"id":"a","label":"a","score":0.5,"topic":"t","path":["t","a"]},
			"topic":{"id":"t","parent":null,"depth":0,"score":0.9},
			"labels":[],"topics":[],"uncertain":true,
			"correction":{"feedback_id":7,"label":{"id":"b","label":"b","score":0.1,"topic":"t","path":["t","b"]},"distance":3},
			"timings_ms":{"total":1.5}}`)
		var c vision.Classification
		strict(t, body, &c)
		if c.Correction == nil || c.Correction.FeedbackId != 7 || c.Correction.Label.Id != "b" || c.Correction.Distance != 3 {
			t.Fatalf("got %+v", c.Correction)
		}
	})

	t.Run("Image - spec example - decodes strictly", func(t *testing.T) {
		var img vision.Image
		strict(t, toJSON(t, upload.Responses["201"].Content[appJSON].Example), &img)
		if img.Id == "" || img.ContentType != "image/jpeg" || img.Width < 1 {
			t.Fatalf("got %+v", img)
		}
	})

	t.Run("CreateClassification - client - posts the image field with the widget bearer", func(t *testing.T) {
		var got imageRequest
		body := toJSON(t, classify.Responses["200"].Content[appJSON].Example)
		client, err := vision.NewClientWithResponses(fakeVision(t, http.StatusOK, body, &got),
			vision.WithRequestEditorFn(widgetBearer))
		if err != nil {
			t.Fatal(err)
		}
		contentType, form := multipartImage(t, photo)
		res, err := client.VisionCreateClassificationWithBodyWithResponse(ctx, contentType, form)
		if err != nil {
			t.Fatal(err)
		}
		if got.method != http.MethodPost || got.path != "/v1/classifications" {
			t.Fatalf("method %q path %q", got.method, got.path)
		}
		if got.auth != "Bearer widget-token" || got.origin != "https://shop.example" || !bytes.Equal(got.image, photo) {
			t.Fatalf("auth %q origin %q image %q", got.auth, got.origin, got.image)
		}
		if res.JSON200 == nil || res.JSON200.Primary.Label != "hut" {
			t.Fatalf("got status %d %+v", res.StatusCode(), res.JSON200)
		}
	})

	t.Run("CreateImage - client - posts the image field and reads the stored image", func(t *testing.T) {
		var got imageRequest
		body := toJSON(t, upload.Responses["201"].Content[appJSON].Example)
		client, err := vision.NewClientWithResponses(fakeVision(t, http.StatusCreated, body, &got),
			vision.WithRequestEditorFn(widgetBearer))
		if err != nil {
			t.Fatal(err)
		}
		contentType, form := multipartImage(t, photo)
		res, err := client.VisionCreateImageWithBodyWithResponse(ctx, contentType, form)
		if err != nil {
			t.Fatal(err)
		}
		if got.path != "/v1/images" || got.auth != "Bearer widget-token" || !bytes.Equal(got.image, photo) {
			t.Fatalf("path %q auth %q image %q", got.path, got.auth, got.image)
		}
		if res.JSON201 == nil || res.JSON201.SizeBytes != 149115 {
			t.Fatalf("got status %d %+v", res.StatusCode(), res.JSON201)
		}
	})

	// Each error status of the two routes reaches its typed field, from the spec example.
	errorCases := []errorCase{
		{http.StatusUnauthorized, "NoWidgetToken"},
		{http.StatusForbidden, "WrongOrigin"},
		{http.StatusRequestEntityTooLarge, "TooLarge"},
		{http.StatusUnsupportedMediaType, "UnsupportedImage"},
		{http.StatusUnprocessableEntity, "Invalid"},
		{http.StatusServiceUnavailable, "Unavailable"},
	}

	t.Run("CreateClassification - each error status - typed error body", func(t *testing.T) {
		for _, tc := range errorCases {
			body := toJSON(t, spec.Components.Responses[tc.response].Content[appJSON].Example)
			client, err := vision.NewClientWithResponses(serveAs(t, tc.status, appJSON, body))
			if err != nil {
				t.Fatal(err)
			}
			contentType, form := multipartImage(t, photo)
			res, err := client.VisionCreateClassificationWithBodyWithResponse(ctx, contentType, form)
			if err != nil {
				t.Fatal(err)
			}
			typed := map[int]bool{
				401: res.JSON401 != nil, 403: res.JSON403 != nil, 413: res.JSON413 != nil,
				415: res.JSON415 != nil, 422: res.JSON422 != nil, 503: res.JSON503 != nil,
			}
			if !typed[tc.status] || res.JSON200 != nil {
				t.Fatalf("%s: status %d not typed", tc.response, tc.status)
			}
		}
	})

	t.Run("CreateImage - each error status - typed error body", func(t *testing.T) {
		cases := slices.Concat(errorCases, []errorCase{{http.StatusInsufficientStorage, "StorageFull"}})
		for _, tc := range cases {
			body := toJSON(t, spec.Components.Responses[tc.response].Content[appJSON].Example)
			client, err := vision.NewClientWithResponses(serveAs(t, tc.status, appJSON, body))
			if err != nil {
				t.Fatal(err)
			}
			contentType, form := multipartImage(t, photo)
			res, err := client.VisionCreateImageWithBodyWithResponse(ctx, contentType, form)
			if err != nil {
				t.Fatal(err)
			}
			typed := map[int]bool{
				401: res.JSON401 != nil, 403: res.JSON403 != nil, 413: res.JSON413 != nil,
				415: res.JSON415 != nil, 422: res.JSON422 != nil, 503: res.JSON503 != nil,
				507: res.JSON507 != nil,
			}
			if !typed[tc.status] || res.JSON201 != nil {
				t.Fatalf("%s: status %d not typed", tc.response, tc.status)
			}
		}
	})
}
