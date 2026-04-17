package youtube

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
)

const (
	baseURL         = "https://www.youtube.com/youtubei/v1/"
	apiKey          = "AIzaSyAO_FJ2SlqU8Q4STEHLGCilw_Y9_11qcW8" // Public API key
	clientName      = "WEB"
	clientVersion   = "2.20260114.08.00"
	userAgent       = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	originURL       = "https://www.youtube.com"
)

// Client is the YouTube innertube API client.  It holds no global state; each
// Client instance is independent.
type Client struct {
	session    Session
	httpClient *http.Client
	logger     *log.Logger
}

// NewClient creates a new Client.  The caller supplies a Session (typically
// loaded via LoadSessionFromCookieFile or LoadSession) and an optional logger.
// If logger is nil, a no-op logger is used.
func NewClient(session Session, logger *log.Logger) *Client {
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}
	if session.Origin == "" {
		session.Origin = originURL
	}
	return &Client{
		session:    session,
		httpClient: &http.Client{},
		logger:     logger,
	}
}

// AddVideoResult holds data returned by a successful AddVideo call.
type AddVideoResult struct {
	VideoID    string
	SetVideoID string
}

// ----------------------------------------------------------------------------
// Public API
// ----------------------------------------------------------------------------

// ListPlaylists returns all playlists in the authenticated user's YouTube
// library, handling pagination automatically.
func (c *Client) ListPlaylists() ([]Playlist, error) {
	body := c.baseContext()
	body["browseId"] = "FEplaylist_aggregation"

	resp, err := c.post("browse", body)
	if err != nil {
		return nil, err
	}

	playlists, token, err := parsePlaylistsResponse(resp)
	if err != nil {
		// Dump the raw response to a file so the structure can be inspected.
		if raw, jerr := json.MarshalIndent(resp, "", "  "); jerr == nil {
			_ = os.WriteFile("debug_playlists_response.json", raw, 0600)
			c.logger.Printf("[youtube] dumped raw playlist response to debug_playlists_response.json")
		}
		return nil, err
	}

	for token != "" {
		contBody := c.baseContext()
		contBody["continuation"] = token

		contResp, err := c.post("browse", contBody)
		if err != nil {
			return nil, fmt.Errorf("fetching playlist continuation page: %w", err)
		}

		more, nextToken, err := parsePlaylistsContinuation(contResp)
		if err != nil {
			return nil, fmt.Errorf("parsing playlist continuation page: %w", err)
		}

		playlists = append(playlists, more...)
		token = nextToken
	}

	return playlists, nil
}

// ListVideos returns the complete list of videos in the given playlist,
// handling all pagination automatically.
func (c *Client) ListVideos(playlistID string) ([]Video, int, error) {
	body := c.baseContext()
	body["browseId"] = "VL" + playlistID

	resp, err := c.post("browse", body)
	if err != nil {
		return nil, 0, err
	}

	// Extract the initial page of videos.
	videos, token, skipped, err := parseVideosResponse(resp, playlistID, c.logger)
	if err != nil {
		return nil, 0, err
	}

	// Follow continuation tokens until exhausted.
	for token != "" {
		contBody := c.baseContext()
		contBody["continuation"] = token

		contResp, err := c.post("browse", contBody)
		if err != nil {
			return nil, 0, fmt.Errorf("fetching continuation page: %w", err)
		}

		more, nextToken, n, err := parseContinuationVideos(contResp, playlistID, c.logger)
		if err != nil {
			return nil, 0, fmt.Errorf("parsing continuation page: %w", err)
		}

		videos = append(videos, more...)
		skipped += n
		token = nextToken
	}

	if skipped > 0 {
		c.logger.Printf("INFO ListVideos %s: %d unavailable videos excluded", playlistID, skipped)
	}
	return videos, skipped, nil
}

// AddVideo adds videoID to the playlist identified by playlistID.
// It logs the action before executing.  Returns an AddVideoResult containing
// the new setVideoId, which can be used immediately for removal without
// re-fetching the playlist.
func (c *Client) AddVideo(playlistID, videoID string) (AddVideoResult, error) {
	c.logger.Printf("[youtube] AddVideo: playlistID=%s videoID=%s", playlistID, videoID)

	body := c.baseContext()
	body["playlistId"] = playlistID
	body["actions"] = []map[string]string{
		{
			"action":       "ACTION_ADD_VIDEO",
			"addedVideoId": videoID,
		},
	}

	resp, err := c.post("browse/edit_playlist", body)
	if err != nil {
		return AddVideoResult{}, err
	}

	return parseAddVideoResponse(resp)
}

// RemoveVideo removes the playlist entry identified by setVideoID from
// playlistID.  The setVideoID must be obtained from a fresh ListVideos call
// immediately before removal — never use a cached value from a previous
// session.
//
// IRREVERSIBLE: removal from YouTube playlist cannot be undone by this app.
func (c *Client) RemoveVideo(playlistID, setVideoID string) error {
	// IRREVERSIBLE: removal from YouTube playlist cannot be undone by this app.
	c.logger.Printf("[youtube] RemoveVideo: playlistID=%s setVideoID=%s", playlistID, setVideoID)

	// Extract the plain videoId from the setVideoId so we can supply
	// removedVideoId.  setVideoId format: "PT:PLxxxxxx:dQw4w9WgXcQ:0:..."
	removedVideoID := videoIDFromSetVideoID(setVideoID)

	body := c.baseContext()
	body["playlistId"] = playlistID
	body["actions"] = []map[string]string{
		{
			"action":         "ACTION_REMOVE_VIDEO",
			"setVideoId":     setVideoID,
			"removedVideoId": removedVideoID,
		},
	}

	resp, err := c.post("browse/edit_playlist", body)
	if err != nil {
		return err
	}

	return parseRemoveVideoResponse(resp)
}

// AddVideos adds multiple videos to a playlist in a single API call.
// This is more efficient than calling AddVideo repeatedly.
func (c *Client) AddVideos(playlistID string, videos []Video) error {
	c.logger.Printf("[youtube] AddVideos: playlistID=%s count=%d", playlistID, len(videos))

	actions := make([]map[string]string, len(videos))
	for i, v := range videos {
		actions[i] = map[string]string{
			"action":       "ACTION_ADD_VIDEO",
			"addedVideoId": v.ID,
		}
	}

	body := c.baseContext()
	body["playlistId"] = playlistID
	body["actions"] = actions

	resp, err := c.post("browse/edit_playlist", body)
	if err != nil {
		return err
	}
	return parseRemoveVideoResponse(resp) // same status-check shape
}

// RemoveVideos removes multiple playlist entries in a single API call.
// This is more efficient than calling RemoveVideo repeatedly and avoids
// concurrent-mutation rejections from YouTube's playlist edit endpoint.
//
// IRREVERSIBLE: removal from YouTube playlist cannot be undone by this app.
func (c *Client) RemoveVideos(playlistID string, videos []Video) error {
	// IRREVERSIBLE: removal from YouTube playlist cannot be undone by this app.
	c.logger.Printf("[youtube] RemoveVideos: playlistID=%s count=%d", playlistID, len(videos))

	actions := make([]map[string]string, len(videos))
	for i, v := range videos {
		actions[i] = map[string]string{
			"action":         "ACTION_REMOVE_VIDEO",
			"setVideoId":     v.SetVideoID,
			"removedVideoId": v.ID,
		}
	}

	body := c.baseContext()
	body["playlistId"] = playlistID
	body["actions"] = actions

	resp, err := c.post("browse/edit_playlist", body)
	if err != nil {
		return err
	}
	return parseRemoveVideoResponse(resp)
}

// FetchVisitorData fetches the YouTube home page to obtain the visitorData
// (X-Goog-Visitor-Id) required for API headers.  The result is stored on the
// session embedded in the client.
func (c *Client) FetchVisitorData() error {
	req, err := http.NewRequest(http.MethodGet, originURL+"/", nil)
	if err != nil {
		return fmt.Errorf("building visitor data request: %w", err)
	}
	c.setCommonHeaders(req)

	httpResp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetching YouTube home page: %w", err)
	}
	defer httpResp.Body.Close()

	bodyBytes, err := readBody(httpResp)
	if err != nil {
		return fmt.Errorf("reading YouTube home page body: %w", err)
	}

	vd, err := extractVisitorData(string(bodyBytes))
	if err != nil {
		return err
	}

	c.session.VisitorData = vd
	return nil
}

// ----------------------------------------------------------------------------
// HTTP helpers
// ----------------------------------------------------------------------------

func (c *Client) post(endpoint string, body map[string]interface{}) (map[string]interface{}, error) {
	url := baseURL + endpoint + "?key=" + apiKey + "&prettyPrint=false"

	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshalling request body: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(encoded))
	if err != nil {
		return nil, fmt.Errorf("building request to %s: %w", endpoint, err)
	}

	c.setCommonHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	httpResp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request to %s: %w", endpoint, err)
	}
	defer httpResp.Body.Close()

	respBytes, err := readBody(httpResp)
	if err != nil {
		return nil, fmt.Errorf("reading response from %s: %w", endpoint, err)
	}

	if err := c.checkHTTPStatus(httpResp.StatusCode, respBytes); err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("parsing JSON response from %s: %w", endpoint, err)
	}

	// Application-level error check: look for {"error": {...}} even on HTTP 200.
	if apiErr := extractAPIError(result); apiErr != nil {
		return nil, apiErr
	}

	return result, nil
}

func (c *Client) setCommonHeaders(req *http.Request) {
	origin := c.session.Origin
	if origin == "" {
		origin = originURL
	}

	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Origin", origin)
	req.Header.Set("X-Origin", origin)
	req.Header.Set("X-Goog-AuthUser", "0")
	req.Header.Set("X-YouTube-Client-Name", "1")
	req.Header.Set("X-YouTube-Client-Version", clientVersion)
	req.Header.Set("X-Youtube-Bootstrap-Logged-In", "true")
	req.Header.Set("Referer", origin+"/")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Encoding", "gzip, deflate")

	if c.session.VisitorData != "" {
		req.Header.Set("X-Goog-Visitor-Id", c.session.VisitorData)
	}

	// Build Authorization header — token value is safe to set in headers but
	// the underlying cookie values are NOT logged.
	if authHeader := BuildAuthHeader(c.session); authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	// Build Cookie header from session cookies.
	if len(c.session.Cookies) > 0 {
		var parts []string
		for k, v := range c.session.Cookies {
			parts = append(parts, k+"="+v)
		}
		req.Header.Set("Cookie", strings.Join(parts, "; "))
	}
}

func (c *Client) checkHTTPStatus(status int, body []byte) error {
	switch {
	case status == http.StatusOK:
		return nil
	case status == http.StatusUnauthorized:
		return &AuthError{HTTPStatus: status, Message: "authentication failed — refresh cookies"}
	case status == http.StatusForbidden:
		return &AuthError{HTTPStatus: status, Message: "forbidden — session may be expired"}
	case status == http.StatusNotFound:
		return &NotFoundError{Resource: "endpoint"}
	case status == 429:
		return &APIError{Code: 429, Message: "rate limited by YouTube", Status: "RATE_LIMITED"}
	case status >= 500:
		return &APIError{Code: status, Message: fmt.Sprintf("server error: HTTP %d", status), Status: "SERVER_ERROR"}
	default:
		// SAFETY WARNING: response body is truncated to 512 chars; verify it never
		// echoes cookie or token values in server-side error pages before expanding
		// this limit or logging to shared infrastructure.
		c.logger.Printf("[youtube] unexpected HTTP status %d body=%s", status, truncate(string(body), 512))
		return &APIError{Code: status, Message: fmt.Sprintf("unexpected HTTP status %d", status), Status: "UNKNOWN"}
	}
}

// ----------------------------------------------------------------------------
// Context / body helpers
// ----------------------------------------------------------------------------

func (c *Client) baseContext() map[string]interface{} {
	return map[string]interface{}{
		"context": map[string]interface{}{
			"client": map[string]interface{}{
				"clientName":      clientName,
				"clientVersion":   clientVersion,
				"hl":              "en",
				"timeZone":        "UTC",
				"utcOffsetMinutes": 0,
			},
			"user":    map[string]interface{}{},
			"request": map[string]interface{}{},
		},
	}
}

// ----------------------------------------------------------------------------
// Response parsers
// ----------------------------------------------------------------------------

// parsePlaylistsResponse extracts playlists from the FEplaylist_aggregation
// browse response and returns any continuation token.
//
// Navigation path (confirmed via YouTube.js source):
//
//	contents
//	  .twoColumnBrowseResultsRenderer
//	  .tabs[0].tabRenderer.content
//	  .sectionListRenderer.contents[0]
//	  .itemSectionRenderer.contents[0]
//	  .gridRenderer.items[]
// parsePlaylistsResponse extracts playlists from the FEplaylist_aggregation
// browse response (richGridRenderer / lockupViewModel shape, current as of 2026).
//
// Path: contents.twoColumnBrowseResultsRenderer.tabs[0].tabRenderer.content
//       .richGridRenderer.contents[].richItemRenderer.content.lockupViewModel
func parsePlaylistsResponse(resp map[string]interface{}) ([]Playlist, string, error) {
	contents, err := navMap(resp, "contents")
	if err != nil {
		return nil, "", fmt.Errorf("ListPlaylists: %w", err)
	}

	twoCol, err := navMap(contents, "twoColumnBrowseResultsRenderer")
	if err != nil {
		return nil, "", fmt.Errorf("ListPlaylists: missing twoColumnBrowseResultsRenderer: %w", err)
	}

	tabs, err := navSlice(twoCol, "tabs")
	if err != nil || len(tabs) == 0 {
		return nil, "", fmt.Errorf("ListPlaylists: missing or empty tabs")
	}

	tab, err := navMap(mustMap(tabs[0]), "tabRenderer")
	if err != nil {
		return nil, "", fmt.Errorf("ListPlaylists: missing tabRenderer: %w", err)
	}

	tabContent, err := navMap(tab, "content")
	if err != nil {
		return nil, "", fmt.Errorf("ListPlaylists: missing tabRenderer.content: %w", err)
	}

	rgr, err := navMap(tabContent, "richGridRenderer")
	if err != nil {
		return nil, "", fmt.Errorf("ListPlaylists: missing richGridRenderer: %w", err)
	}

	return extractPlaylistsFromRichGrid(rgr)
}

// parsePlaylistsContinuation extracts playlists from a continuation response.
func parsePlaylistsContinuation(resp map[string]interface{}) ([]Playlist, string, error) {
	// Try richGridContinuation first (current shape), fall back to gridContinuation.
	if cc, ok := resp["continuationContents"].(map[string]interface{}); ok {
		if rgc, ok := cc["richGridContinuation"].(map[string]interface{}); ok {
			return extractPlaylistsFromRichGrid(rgc)
		}
		if gc, ok := cc["gridContinuation"].(map[string]interface{}); ok {
			return extractPlaylistsFromRichGrid(gc)
		}
	}
	return nil, "", fmt.Errorf("ListPlaylists continuation: unrecognised continuation shape")
}

// extractPlaylistsFromRichGrid extracts lockupViewModel playlist items and any
// continuation token from a richGridRenderer or richGridContinuation map.
func extractPlaylistsFromRichGrid(grid map[string]interface{}) ([]Playlist, string, error) {
	items, err := navSlice(grid, "contents")
	if err != nil {
		return nil, "", fmt.Errorf("richGridRenderer missing contents: %w", err)
	}

	var playlists []Playlist
	var token string

	for _, rawItem := range items {
		item := mustMap(rawItem)

		// Continuation token.
		if cont, ok := item["continuationItemRenderer"].(map[string]interface{}); ok {
			_ = walkToken(cont, &token)
			continue
		}

		ri, ok := item["richItemRenderer"].(map[string]interface{})
		if !ok {
			continue
		}
		content, ok := ri["content"].(map[string]interface{})
		if !ok {
			continue
		}
		lvm, ok := content["lockupViewModel"].(map[string]interface{})
		if !ok {
			continue
		}
		// Only process playlist items.
		if ct, _ := lvm["contentType"].(string); ct != "LOCKUP_CONTENT_TYPE_PLAYLIST" {
			continue
		}

		pl, err := parseLockupViewModel(lvm)
		if err != nil {
			return nil, "", fmt.Errorf("parsing lockupViewModel: %w", err)
		}
		playlists = append(playlists, pl)
	}

	return playlists, token, nil
}

// walkToken is a best-effort recursive search for a continuation token string.
// It recurses into both maps and slices.
func walkToken(m map[string]interface{}, out *string) bool {
	for k, v := range m {
		if k == "token" {
			if s, ok := v.(string); ok && s != "" {
				*out = s
				return true
			}
		}
		if walkValue(v, out) {
			return true
		}
	}
	return false
}

func walkValue(v interface{}, out *string) bool {
	switch val := v.(type) {
	case map[string]interface{}:
		return walkToken(val, out)
	case []interface{}:
		for _, elem := range val {
			if walkValue(elem, out) {
				return true
			}
		}
	}
	return false
}

// parseLockupViewModel extracts a Playlist from the lockupViewModel shape
// introduced in 2025/2026.
//
// ID:    lockupViewModel.contentId
// Title: lockupViewModel.metadata.lockupMetadataViewModel.title.content
// Count: badge text inside contentImage overlays, e.g. "3,707 videos"
func parseLockupViewModel(lvm map[string]interface{}) (Playlist, error) {
	id, _ := lvm["contentId"].(string)
	if id == "" {
		return Playlist{}, fmt.Errorf("lockupViewModel missing contentId")
	}

	// Title.
	title := ""
	if meta, ok := lvm["metadata"].(map[string]interface{}); ok {
		if lmvm, ok := meta["lockupMetadataViewModel"].(map[string]interface{}); ok {
			if t, ok := lmvm["title"].(map[string]interface{}); ok {
				title, _ = t["content"].(string)
			}
		}
	}

	// Video count — buried in thumbnail overlay badge text, e.g. "3,707 videos".
	count := 0
	if ci, ok := lvm["contentImage"].(map[string]interface{}); ok {
		if ctvm, ok := ci["collectionThumbnailViewModel"].(map[string]interface{}); ok {
			if pt, ok := ctvm["primaryThumbnail"].(map[string]interface{}); ok {
				if tvm, ok := pt["thumbnailViewModel"].(map[string]interface{}); ok {
					if overlays, ok := tvm["overlays"].([]interface{}); ok {
						for _, ov := range overlays {
							ovMap := mustMap(ov)
							if badge, ok := ovMap["thumbnailOverlayBadgeViewModel"].(map[string]interface{}); ok {
								if badges, ok := badge["thumbnailBadges"].([]interface{}); ok && len(badges) > 0 {
									if bvm, ok := mustMap(badges[0])["thumbnailBadgeViewModel"].(map[string]interface{}); ok {
										if text, _ := bvm["text"].(string); text != "" {
											fields := strings.Fields(strings.ReplaceAll(text, ",", ""))
											if len(fields) > 0 {
												count, _ = strconv.Atoi(fields[0])
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}

	return Playlist{ID: id, Title: title, Count: count}, nil
}

// parseGridPlaylistRenderer is kept for any legacy gridPlaylistRenderer items
// that may still appear in continuation responses.
func parseGridPlaylistRenderer(r map[string]interface{}) (Playlist, error) {
	id, err := navString(r, "playlistId")
	if err != nil {
		return Playlist{}, fmt.Errorf("gridPlaylistRenderer missing playlistId: %w", err)
	}

	titleRuns, err := navSlice(mustMapAt(r, "title"), "runs")
	if err != nil || len(titleRuns) == 0 {
		return Playlist{}, fmt.Errorf("gridPlaylistRenderer missing title.runs for playlist %s", id)
	}
	title, _ := navString(mustMap(titleRuns[0]), "text")

	count := 0
	for _, fieldName := range []string{"thumbnailText", "videoCountText", "videoCountShortText"} {
		vcText, ok := r[fieldName].(map[string]interface{})
		if !ok {
			continue
		}
		vcRuns, err := navSlice(vcText, "runs")
		if err != nil || len(vcRuns) == 0 {
			continue
		}
		// Concatenate all run texts then parse the leading number.
		var sb strings.Builder
		for _, run := range vcRuns {
			if t, _ := navString(mustMap(run), "text"); t != "" {
				sb.WriteString(t)
			}
		}
		fields := strings.Fields(sb.String())
		if len(fields) > 0 {
			count, _ = strconv.Atoi(strings.ReplaceAll(fields[0], ",", ""))
		}
		if count > 0 {
			break
		}
	}

	return Playlist{ID: id, Title: title, Count: count}, nil
}

// parseVideosResponse extracts videos from the initial browse response and
// returns any continuation token.
func parseVideosResponse(resp map[string]interface{}, playlistID string, logger *log.Logger) ([]Video, string, int, error) {
	// Path: .contents.twoColumnBrowseResultsRenderer.tabs[0].tabRenderer.content
	//       .sectionListRenderer.contents[0].itemSectionRenderer.contents[0]
	//       .playlistVideoListRenderer.contents[]

	contents, err := navMap(resp, "contents")
	if err != nil {
		return nil, "", 0, fmt.Errorf("ListVideos: %w", err)
	}

	twoCol, err := navMap(contents, "twoColumnBrowseResultsRenderer")
	if err != nil {
		return nil, "", 0, fmt.Errorf("ListVideos: missing twoColumnBrowseResultsRenderer: %w", err)
	}

	tabs, err := navSlice(twoCol, "tabs")
	if err != nil || len(tabs) == 0 {
		return nil, "", 0, fmt.Errorf("ListVideos: missing tabs in twoColumnBrowseResultsRenderer")
	}

	tab0, err := navMap(mustMap(tabs[0]), "tabRenderer")
	if err != nil {
		return nil, "", 0, fmt.Errorf("ListVideos: missing tabRenderer: %w", err)
	}

	tabContent, err := navMap(tab0, "content")
	if err != nil {
		return nil, "", 0, fmt.Errorf("ListVideos: missing tab content: %w", err)
	}

	slr, err := navMap(tabContent, "sectionListRenderer")
	if err != nil {
		return nil, "", 0, fmt.Errorf("ListVideos: missing sectionListRenderer: %w", err)
	}

	slrContents, err := navSlice(slr, "contents")
	if err != nil || len(slrContents) == 0 {
		return nil, "", 0, fmt.Errorf("ListVideos: sectionListRenderer.contents empty or missing")
	}

	isr, err := navMap(mustMap(slrContents[0]), "itemSectionRenderer")
	if err != nil {
		return nil, "", 0, fmt.Errorf("ListVideos: missing itemSectionRenderer: %w", err)
	}

	isrContents, err := navSlice(isr, "contents")
	if err != nil || len(isrContents) == 0 {
		return nil, "", 0, fmt.Errorf("ListVideos: itemSectionRenderer.contents empty or missing")
	}

	pvlr, err := navMap(mustMap(isrContents[0]), "playlistVideoListRenderer")
	if err != nil {
		return nil, "", 0, fmt.Errorf("ListVideos: missing playlistVideoListRenderer: %w", err)
	}

	pvlrContents, err := navSlice(pvlr, "contents")
	if err != nil {
		return nil, "", 0, fmt.Errorf("ListVideos: playlistVideoListRenderer.contents missing: %w", err)
	}

	return extractVideosFromItems(pvlrContents, playlistID, logger)
}

// parseContinuationVideos extracts videos from a continuation response.
// Primary path: .onResponseReceivedActions[0].appendContinuationItemsAction.continuationItems[]
// Fallback path: .continuationContents.playlistVideoListContinuation.contents[]
func parseContinuationVideos(resp map[string]interface{}, playlistID string, logger *log.Logger) ([]Video, string, int, error) {
	// Collect any error alerts for logging, but do NOT return early — YouTube
	// sometimes sends an alert alongside the final batch of items (seen with
	// Watch Later). We try to extract items first and only give up if we
	// genuinely cannot find any.
	var alertMsgs []string
	if alerts, err := navSlice(resp, "alerts"); err == nil {
		for _, a := range alerts {
			am := mustMap(a)
			if ar, ok := am["alertRenderer"].(map[string]interface{}); ok {
				if t, _ := ar["type"].(string); t == "ERROR" {
					if textMap, ok := ar["text"].(map[string]interface{}); ok {
						if runs, err2 := navSlice(textMap, "runs"); err2 == nil && len(runs) > 0 {
							if txt, _ := mustMap(runs[0])["text"].(string); txt != "" {
								alertMsgs = append(alertMsgs, txt)
							}
						}
					}
				}
			}
		}
	}

	// Primary path.
	if actions, err := navSlice(resp, "onResponseReceivedActions"); err == nil && len(actions) > 0 {
		action := mustMap(actions[0])
		if acia, err := navMap(action, "appendContinuationItemsAction"); err == nil {
			if items, err := navSlice(acia, "continuationItems"); err == nil {
				return extractVideosFromItems(items, playlistID, logger)
			}
		}
	}

	// Fallback path (older API versions).
	if cc, err := navMap(resp, "continuationContents"); err == nil {
		if pvlc, err := navMap(cc, "playlistVideoListContinuation"); err == nil {
			if items, err := navSlice(pvlc, "contents"); err == nil {
				return extractVideosFromItems(items, playlistID, logger)
			}
		}
	}

	// No items found. If there were alerts, treat as end-of-list (common for
	// Watch Later when YouTube refuses further pagination).
	if len(alertMsgs) > 0 {
		logger.Printf("WARN pagination ended for playlist %s: %s", playlistID, strings.Join(alertMsgs, "; "))
		return nil, "", 0, nil
	}

	if raw, jerr := json.MarshalIndent(resp, "", "  "); jerr == nil {
		_ = os.WriteFile("debug_continuation_resp.json", raw, 0600)
	}
	return nil, "", 0, fmt.Errorf("ListVideos continuation: could not find continuationItems in response")
}

// extractVideosFromItems processes a []interface{} of raw video items,
// returning parsed Videos, any continuation token for the next page, and the
// number of unavailable entries that were skipped.
func extractVideosFromItems(items []interface{}, playlistID string, logger *log.Logger) ([]Video, string, int, error) {
	var videos []Video
	var nextToken string
	skipped := 0

	for _, rawItem := range items {
		item := mustMap(rawItem)

		// Continuation sentinel.
		if cont, ok := item["continuationItemRenderer"].(map[string]interface{}); ok {
			token, err := extractContinuationToken(cont)
			if err != nil {
				// Dump the raw continuationItemRenderer so we can inspect the real structure.
				if raw, jerr := json.MarshalIndent(cont, "", "  "); jerr == nil {
					_ = os.WriteFile("debug_continuation.json", raw, 0600)
				}
				return nil, "", 0, fmt.Errorf("extracting continuation token: %w", err)
			}
			nextToken = token
			continue
		}

		renderer, ok := item["playlistVideoRenderer"].(map[string]interface{})
		if !ok {
			// Unknown item type — skip.
			continue
		}

		v, err := parsePlaylistVideoRenderer(renderer, playlistID)
		if err != nil {
			logger.Printf("WARN skipping video: %v", err)
			skipped++
			continue
		}
		if v.Unavailable {
			logger.Printf("WARN skipping unavailable video %s (%s): %s", v.Title, v.ID, v.UnavailableReason)
			skipped++
			continue
		}
		videos = append(videos, v)
	}

	return videos, nextToken, skipped, nil
}

// brokenTitles are the placeholder strings YouTube uses for unavailable videos.
var brokenTitles = []string{
	"[private video]",
	"[deleted video]",
	"[unavailable video]",
	"[video unavailable]",
	"[removed video]",
}

func parsePlaylistVideoRenderer(r map[string]interface{}, playlistID string) (Video, error) {
	videoID, err := navString(r, "videoId")
	if err != nil {
		return Video{}, fmt.Errorf("playlistVideoRenderer missing videoId: %w", err)
	}

	// setVideoId is REQUIRED for removal.  Prefer the top-level field; fall
	// back to the menu path used by ytmusicapi.
	setVideoID, _ := navString(r, "setVideoId")
	if setVideoID == "" {
		setVideoID = extractSetVideoIDFromMenu(r)
	}

	// Title.
	title := ""
	if titleMap, ok := r["title"].(map[string]interface{}); ok {
		if runs, err := navSlice(titleMap, "runs"); err == nil && len(runs) > 0 {
			title, _ = navString(mustMap(runs[0]), "text")
		}
	}

	// Channel name.
	channel := ""
	if byline, ok := r["shortBylineText"].(map[string]interface{}); ok {
		if runs, err := navSlice(byline, "runs"); err == nil && len(runs) > 0 {
			channel, _ = navString(mustMap(runs[0]), "text")
		}
	}

	// Duration.
	duration := ""
	if lenText, ok := r["lengthText"].(map[string]interface{}); ok {
		duration, _ = navString(lenText, "simpleText")
	}

	// YouTube sets isPlayable=false for deleted/private/region-blocked videos.
	isPlayable := true
	if v, ok := r["isPlayable"].(bool); ok {
		isPlayable = v
	}

	// Detect unavailability: missing setVideoId, known broken title, no channel+duration, or not playable.
	unavailable, reason := detectUnavailable(videoID, setVideoID, title, channel, duration, isPlayable)

	return Video{
		ID:                videoID,
		SetVideoID:        setVideoID,
		Title:             title,
		Channel:           channel,
		Duration:          duration,
		PlaylistID:        playlistID,
		Unavailable:       unavailable,
		UnavailableReason: reason,
	}, nil
}

// detectUnavailable returns true when a video entry appears to be broken
// (private, deleted, region-blocked, or otherwise inaccessible).
func detectUnavailable(videoID, setVideoID, title, channel, duration string, isPlayable bool) (bool, string) {
	// YouTube's own playability flag is the most reliable signal.
	if !isPlayable {
		return true, "not playable (deleted, private, or region-blocked)"
	}
	lower := strings.ToLower(strings.TrimSpace(title))
	for _, bad := range brokenTitles {
		if lower == bad {
			return true, title
		}
	}
	if setVideoID == "" {
		return true, "missing playlist entry ID (private or deleted)"
	}
	// No channel + no duration means the video has been deleted or made private
	// but YouTube still holds a slot for it in the playlist.
	if channel == "" && duration == "" {
		return true, "no metadata (deleted or private)"
	}
	return false, ""
}

// extractSetVideoIDFromMenu walks the menu renderer to find setVideoId as a
// fallback when it is absent at the top level.
func extractSetVideoIDFromMenu(r map[string]interface{}) string {
	menu, ok := r["menu"].(map[string]interface{})
	if !ok {
		return ""
	}
	mr, ok := menu["menuRenderer"].(map[string]interface{})
	if !ok {
		return ""
	}
	items, _ := navSlice(mr, "items")
	for _, rawMI := range items {
		mi := mustMap(rawMI)
		msir, ok := mi["menuServiceItemRenderer"].(map[string]interface{})
		if !ok {
			continue
		}
		se, ok := msir["serviceEndpoint"].(map[string]interface{})
		if !ok {
			continue
		}
		pee, ok := se["playlistEditEndpoint"].(map[string]interface{})
		if !ok {
			continue
		}
		actions, _ := navSlice(pee, "actions")
		if len(actions) == 0 {
			continue
		}
		if sv, _ := navString(mustMap(actions[0]), "setVideoId"); sv != "" {
			return sv
		}
	}
	return ""
}

// extractContinuationToken pulls the token string from a continuationItemRenderer.
// It tries the canonical path first, then falls back to a recursive search.
func extractContinuationToken(cont map[string]interface{}) (string, error) {
	// Canonical path: continuationEndpoint.continuationCommand.token
	if ce, ok := cont["continuationEndpoint"].(map[string]interface{}); ok {
		if cc, ok := ce["continuationCommand"].(map[string]interface{}); ok {
			if t, _ := cc["token"].(string); t != "" {
				return t, nil
			}
		}
	}
	// Fallback: recursive search for any "token" string key.
	var token string
	if walkToken(cont, &token) {
		return token, nil
	}
	return "", fmt.Errorf("no continuation token found in continuationItemRenderer")
}

func parseAddVideoResponse(resp map[string]interface{}) (AddVideoResult, error) {
	status, _ := navString(resp, "status")
	if status != "STATUS_SUCCEEDED" {
		if status == "" {
			status = "<missing>"
		}
		return AddVideoResult{}, &MutationError{Operation: "AddVideo", Status: status}
	}

	results, err := navSlice(resp, "playlistEditResults")
	if err != nil || len(results) == 0 {
		return AddVideoResult{}, fmt.Errorf("AddVideo: response missing playlistEditResults")
	}

	first := mustMap(results[0])
	added, ok := first["playlistEditVideoAddedResultData"].(map[string]interface{})
	if !ok {
		return AddVideoResult{}, fmt.Errorf("AddVideo: response missing playlistEditVideoAddedResultData")
	}

	videoID, _ := navString(added, "videoId")
	setVideoID, _ := navString(added, "setVideoId")

	if setVideoID == "" {
		return AddVideoResult{}, fmt.Errorf("AddVideo: response missing setVideoId in playlistEditVideoAddedResultData")
	}

	return AddVideoResult{VideoID: videoID, SetVideoID: setVideoID}, nil
}

func parseRemoveVideoResponse(resp map[string]interface{}) error {
	status, _ := navString(resp, "status")
	if status != "STATUS_SUCCEEDED" {
		if status == "" {
			status = "<missing>"
		}
		return &MutationError{Operation: "RemoveVideo", Status: status}
	}
	return nil
}

// ScanBrokenVideos fetches all entries from a playlist and returns those that
// appear to be unavailable (private, deleted, region-blocked, etc.).
// Unlike ListVideos, it does NOT skip unavailable entries — it collects them.
// The second return value is truncatedCount: how many videos could not be
// scanned due to pagination limits (expectedCount - totalFetched, clamped to 0).
func (c *Client) ScanBrokenVideos(playlistID string, expectedCount int) ([]Video, int, error) {
	body := c.baseContext()
	body["browseId"] = "VL" + playlistID

	resp, err := c.post("browse", body)
	if err != nil {
		return nil, 0, err
	}

	all, token, err := parseVideosResponseAll(resp, playlistID, c.logger)
	if err != nil {
		return nil, 0, err
	}
	totalFetched := len(all)

	for token != "" {
		contBody := c.baseContext()
		contBody["continuation"] = token

		contResp, err := c.post("browse", contBody)
		if err != nil {
			return nil, 0, fmt.Errorf("fetching continuation page: %w", err)
		}

		more, nextToken, err := parseContinuationVideosAll(contResp, playlistID, c.logger)
		if err != nil {
			return nil, 0, fmt.Errorf("parsing continuation page: %w", err)
		}

		all = append(all, more...)
		totalFetched += len(more)
		token = nextToken
	}

	truncatedCount := 0
	if expectedCount > 0 {
		truncatedCount = expectedCount - totalFetched
		if truncatedCount < 0 {
			truncatedCount = 0
		}
	}

	var broken []Video
	for _, v := range all {
		if v.Unavailable {
			broken = append(broken, v)
		}
	}
	return broken, truncatedCount, nil
}

// parseVideosResponseAll is like parseVideosResponse but keeps unavailable videos.
func parseVideosResponseAll(resp map[string]interface{}, playlistID string, logger *log.Logger) ([]Video, string, error) {
	contents, err := navMap(resp, "contents")
	if err != nil {
		return nil, "", fmt.Errorf("ScanBrokenVideos: %w", err)
	}
	twoCol, err := navMap(contents, "twoColumnBrowseResultsRenderer")
	if err != nil {
		return nil, "", fmt.Errorf("ScanBrokenVideos: missing twoColumnBrowseResultsRenderer: %w", err)
	}
	tabs, err := navSlice(twoCol, "tabs")
	if err != nil || len(tabs) == 0 {
		return nil, "", fmt.Errorf("ScanBrokenVideos: missing tabs")
	}
	tab0, err := navMap(mustMap(tabs[0]), "tabRenderer")
	if err != nil {
		return nil, "", fmt.Errorf("ScanBrokenVideos: missing tabRenderer: %w", err)
	}
	tabContent, err := navMap(tab0, "content")
	if err != nil {
		return nil, "", fmt.Errorf("ScanBrokenVideos: missing tab content: %w", err)
	}
	slr, err := navMap(tabContent, "sectionListRenderer")
	if err != nil {
		return nil, "", fmt.Errorf("ScanBrokenVideos: missing sectionListRenderer: %w", err)
	}
	slrContents, err := navSlice(slr, "contents")
	if err != nil || len(slrContents) == 0 {
		return nil, "", fmt.Errorf("ScanBrokenVideos: sectionListRenderer.contents empty")
	}
	isr, err := navMap(mustMap(slrContents[0]), "itemSectionRenderer")
	if err != nil {
		return nil, "", fmt.Errorf("ScanBrokenVideos: missing itemSectionRenderer: %w", err)
	}
	isrContents, err := navSlice(isr, "contents")
	if err != nil || len(isrContents) == 0 {
		return nil, "", fmt.Errorf("ScanBrokenVideos: itemSectionRenderer.contents empty")
	}
	pvlr, err := navMap(mustMap(isrContents[0]), "playlistVideoListRenderer")
	if err != nil {
		return nil, "", fmt.Errorf("ScanBrokenVideos: missing playlistVideoListRenderer: %w", err)
	}
	pvlrContents, err := navSlice(pvlr, "contents")
	if err != nil {
		return nil, "", fmt.Errorf("ScanBrokenVideos: playlistVideoListRenderer.contents missing: %w", err)
	}
	return extractVideosFromItemsAll(pvlrContents, playlistID, logger)
}

// parseContinuationVideosAll is like parseContinuationVideos but keeps unavailable videos.
func parseContinuationVideosAll(resp map[string]interface{}, playlistID string, logger *log.Logger) ([]Video, string, error) {
	// Collect alerts for logging but try items first (mirrors parseContinuationVideos).
	var alertMsgs []string
	if alerts, err := navSlice(resp, "alerts"); err == nil {
		for _, a := range alerts {
			am := mustMap(a)
			if ar, ok := am["alertRenderer"].(map[string]interface{}); ok {
				if t, _ := ar["type"].(string); t == "ERROR" {
					if textMap, ok := ar["text"].(map[string]interface{}); ok {
						if runs, err2 := navSlice(textMap, "runs"); err2 == nil && len(runs) > 0 {
							if txt, _ := mustMap(runs[0])["text"].(string); txt != "" {
								alertMsgs = append(alertMsgs, txt)
							}
						}
					}
				}
			}
		}
	}

	if actions, err := navSlice(resp, "onResponseReceivedActions"); err == nil && len(actions) > 0 {
		action := mustMap(actions[0])
		if acia, err := navMap(action, "appendContinuationItemsAction"); err == nil {
			if items, err := navSlice(acia, "continuationItems"); err == nil {
				return extractVideosFromItemsAll(items, playlistID, logger)
			}
		}
	}
	if cc, err := navMap(resp, "continuationContents"); err == nil {
		if pvlc, err := navMap(cc, "playlistVideoListContinuation"); err == nil {
			if items, err := navSlice(pvlc, "contents"); err == nil {
				return extractVideosFromItemsAll(items, playlistID, logger)
			}
		}
	}
	if len(alertMsgs) > 0 {
		logger.Printf("WARN scan pagination ended for playlist %s: %s", playlistID, strings.Join(alertMsgs, "; "))
	}
	return nil, "", nil
}

// extractVideosFromItemsAll is like extractVideosFromItems but includes unavailable videos.
func extractVideosFromItemsAll(items []interface{}, playlistID string, logger *log.Logger) ([]Video, string, error) {
	var videos []Video
	var nextToken string

	for _, rawItem := range items {
		item := mustMap(rawItem)

		if cont, ok := item["continuationItemRenderer"].(map[string]interface{}); ok {
			token, err := extractContinuationToken(cont)
			if err == nil && token != "" {
				nextToken = token
			}
			continue
		}

		renderer, ok := item["playlistVideoRenderer"].(map[string]interface{})
		if !ok {
			// Log unknown item types so they're visible in the log.
			for k := range item {
				if k != "continuationItemRenderer" {
					logger.Printf("WARN scan: unrecognised item type %q — skipped", k)
				}
			}
			continue
		}

		v, err := parsePlaylistVideoRenderer(renderer, playlistID)
		if err != nil {
			// Can't parse the renderer fully (e.g. missing videoId). Record a
			// stub so it still appears in the broken-videos list.
			videoID, _ := navString(renderer, "videoId")
			if videoID == "" {
				videoID = "(unknown)"
			}
			logger.Printf("WARN scan: could not parse video %s: %v", videoID, err)
			videos = append(videos, Video{
				ID:                videoID,
				PlaylistID:        playlistID,
				Unavailable:       true,
				UnavailableReason: fmt.Sprintf("parse error: %v", err),
			})
			continue
		}
		videos = append(videos, v)
	}

	return videos, nextToken, nil
}

// DeletePlaylist permanently deletes the playlist identified by playlistID.
//
// IRREVERSIBLE: the playlist and all its entries are permanently removed from
// the user's YouTube account. This cannot be undone by this app.
func (c *Client) DeletePlaylist(playlistID string) error {
	// IRREVERSIBLE: playlist deletion cannot be undone by this app.
	c.logger.Printf("[youtube] DeletePlaylist: playlistID=%s", playlistID)

	body := c.baseContext()
	body["playlistId"] = playlistID

	// playlist/delete returns {} or just responseContext on success —
	// no "status" field. post() already handles HTTP errors and API error objects.
	_, err := c.post("playlist/delete", body)
	return err
}

// ----------------------------------------------------------------------------
// Visitor data extraction
// ----------------------------------------------------------------------------

var ytcfgRe = regexp.MustCompile(`ytcfg\.set\s*\(\s*(\{.+?\})\s*\)\s*;`)
var visitorDataRe = regexp.MustCompile(`"VISITOR_DATA"\s*:\s*"([^"]+)"`)

func extractVisitorData(html string) (string, error) {
	// Fast path: look for the VISITOR_DATA key directly.
	if m := visitorDataRe.FindStringSubmatch(html); len(m) == 2 {
		return m[1], nil
	}

	// Slower path: extract ytcfg.set({...}) and JSON-parse it.
	matches := ytcfgRe.FindAllStringSubmatch(html, -1)
	for _, m := range matches {
		var cfg map[string]interface{}
		if err := json.Unmarshal([]byte(m[1]), &cfg); err != nil {
			continue
		}
		if vd, ok := cfg["VISITOR_DATA"].(string); ok && vd != "" {
			return vd, nil
		}
	}

	return "", fmt.Errorf("could not extract VISITOR_DATA from YouTube home page")
}

// ----------------------------------------------------------------------------
// API error extraction
// ----------------------------------------------------------------------------

func extractAPIError(resp map[string]interface{}) error {
	errObj, ok := resp["error"].(map[string]interface{})
	if !ok {
		return nil
	}

	code := 0
	if c, ok := errObj["code"].(float64); ok {
		code = int(c)
	}
	message, _ := errObj["message"].(string)
	status, _ := errObj["status"].(string)

	return &APIError{Code: code, Message: message, Status: status}
}

// ----------------------------------------------------------------------------
// Navigation helpers — typed accessors for map[string]interface{} trees
// ----------------------------------------------------------------------------

func navMap(m map[string]interface{}, key string) (map[string]interface{}, error) {
	v, ok := m[key]
	if !ok {
		return nil, fmt.Errorf("missing key %q", key)
	}
	r, ok := v.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("key %q is not an object (got %T)", key, v)
	}
	return r, nil
}

func navSlice(m map[string]interface{}, key string) ([]interface{}, error) {
	v, ok := m[key]
	if !ok {
		return nil, fmt.Errorf("missing key %q", key)
	}
	r, ok := v.([]interface{})
	if !ok {
		return nil, fmt.Errorf("key %q is not an array (got %T)", key, v)
	}
	return r, nil
}

func navString(m map[string]interface{}, key string) (string, error) {
	v, ok := m[key]
	if !ok {
		return "", fmt.Errorf("missing key %q", key)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("key %q is not a string (got %T)", key, v)
	}
	return s, nil
}

// mustMap coerces an interface{} to map[string]interface{}; returns empty map
// on failure so callers can continue navigating without panicking.
func mustMap(v interface{}) map[string]interface{} {
	if m, ok := v.(map[string]interface{}); ok {
		return m
	}
	return map[string]interface{}{}
}

// mustMapAt retrieves a nested map by key, returning an empty map if absent.
func mustMapAt(m map[string]interface{}, key string) map[string]interface{} {
	if v, ok := m[key].(map[string]interface{}); ok {
		return v
	}
	return map[string]interface{}{}
}

// ----------------------------------------------------------------------------
// Misc helpers
// ----------------------------------------------------------------------------

// videoIDFromSetVideoID extracts the plain video ID from a setVideoId string.
// Format: "PT:PLxxxxxx:dQw4w9WgXcQ:0:1234567890"
func videoIDFromSetVideoID(setVideoID string) string {
	parts := strings.Split(setVideoID, ":")
	if len(parts) >= 3 {
		return parts[2]
	}
	return ""
}

// readBody reads an HTTP response body, transparently decompressing gzip.
func readBody(resp *http.Response) ([]byte, error) {
	var reader io.Reader = resp.Body
	if strings.EqualFold(resp.Header.Get("Content-Encoding"), "gzip") {
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("creating gzip reader: %w", err)
		}
		defer gz.Close()
		reader = gz
	}
	return io.ReadAll(reader)
}

// truncate returns s truncated to at most n runes (for log safety).
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}
