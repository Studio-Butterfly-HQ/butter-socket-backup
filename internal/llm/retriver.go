package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
)

// ── Config ────────────────────────────────────────────────────────────────────

var (
	qdrantURL      = getEnv("QDRANT_URL", "http://localhost:6333")
	qdrantAPIKey   = getEnv("QDRANT_API_KEY", "")
	embeddingModel = "text-embedding-3-large"
	topK           = 8    // how many chunks to retrieve
	minScore       = 0.35 // relevance threshold — below this = no useful match
)

// ErrCollectionNotFound is returned when the company has no Qdrant collection yet.
var ErrCollectionNotFound = fmt.Errorf("knowledge base not found for this company")

// greetingPhrases catches common greetings in any language so we respond warmly
// instead of treating them as irrelevant queries.
var greetingPhrases = []string{
	"hi", "hello", "hey", "hiya", "howdy",
	"good morning", "good afternoon", "good evening", "good night",
	"salam", "salaam", "assalamu alaikum", "assalamualaikum",
	"আস্সালামু আলাইকুম", "সালাম", "হ্যালো", "হ্যা", "হেলো",
	"নমস্কার", "নমস্তে", "কেমন আছেন", "কি খবর",
	"bonjour", "hola", "ciao", "مرحبا", "السلام عليكم",
}

func isGreeting(query string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	for _, g := range greetingPhrases {
		if q == g || strings.HasPrefix(q, g+" ") || strings.HasSuffix(q, " "+g) {
			return true
		}
	}
	return false
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// ── Qdrant types ──────────────────────────────────────────────────────────────

type qdrantSearchRequest struct {
	Vector         []float64 `json:"vector"`
	Limit          int       `json:"limit"`
	WithPayload    bool      `json:"with_payload"`
	ScoreThreshold float64   `json:"score_threshold"`
}

type qdrantPoint struct {
	ID      string                 `json:"id"`
	Score   float64                `json:"score"`
	Payload map[string]interface{} `json:"payload"`
}

type qdrantSearchResponse struct {
	Result []qdrantPoint `json:"result"`
	Status string        `json:"status"`
}

// ── RetrievedChunk ────────────────────────────────────────────────────────────

type RetrievedChunk struct {
	Text        string
	Score       float64
	SectionPath string
	Intent      string
	SourceType  string
	FileName    string
	PageNumber  interface{}
	URI         string // extracted URL from text if present
}

// ── RAG Response ──────────────────────────────────────────────────────────────

type RAGResult struct {
	Answer   string
	Chunks   []RetrievedChunk
	Relevant bool
	HasData  bool
}

// ── Public API ────────────────────────────────────────────────────────────────

// RetrieveAndAnswer performs full RAG: embed → search → prompt → stream answer.
// onToken is called for each streamed token. Returns the full answer when done.
func RetrieveAndAnswer(
	ctx context.Context,
	companyID string,
	userQuery string,
	onToken func(token string),
) (RAGResult, error) {

	// 1. Embed the user query
	embedding, err := embedQuery(ctx, userQuery)
	if err != nil {
		return RAGResult{}, fmt.Errorf("embedding query: %w", err)
	}

	// 2. Detect greeting early — no need to hit Qdrant
	if isGreeting(userQuery) {
		prompt := buildGreetingPrompt(userQuery)
		var fullAnswer strings.Builder
		err = streamAnswer(ctx, prompt, func(token string) {
			fullAnswer.WriteString(token)
			if onToken != nil {
				onToken(token)
			}
		})
		if err != nil {
			return RAGResult{}, fmt.Errorf("streaming greeting: %w", err)
		}
		return RAGResult{Answer: fullAnswer.String(), Relevant: true, HasData: true}, nil
	}

	// 3. Search Qdrant for the company's collection
	chunks, err := searchQdrant(ctx, companyID, embedding)
	if err != nil {
		if err == ErrCollectionNotFound {
			prompt := buildNoCollectionPrompt(userQuery)
			var fullAnswer strings.Builder
			_ = streamAnswer(ctx, prompt, func(token string) {
				fullAnswer.WriteString(token)
				if onToken != nil {
					onToken(token)
				}
			})
			return RAGResult{
				Answer:   fullAnswer.String(),
				Relevant: false,
				HasData:  false,
			}, nil
		}
		return RAGResult{}, fmt.Errorf("qdrant search: %w", err)
	}

	// 4. Evaluate relevance
	relevant, hasData := evaluateRelevance(chunks)

	// 5. Build prompt based on relevance
	prompt := buildPrompt(userQuery, chunks, relevant, hasData)

	// 5. Stream response
	var fullAnswer strings.Builder
	err = streamAnswer(ctx, prompt, func(token string) {
		fullAnswer.WriteString(token)
		if onToken != nil {
			onToken(token)
		}
	})
	if err != nil {
		return RAGResult{}, fmt.Errorf("streaming answer: %w", err)
	}

	return RAGResult{
		Answer:   fullAnswer.String(),
		Chunks:   chunks,
		Relevant: relevant,
		HasData:  hasData,
	}, nil
}

// RetrieveAndAnswerSync is the non-streaming version. Returns the full answer.
func RetrieveAndAnswerSync(
	ctx context.Context,
	companyID string,
	userQuery string,
) (RAGResult, error) {
	return RetrieveAndAnswer(ctx, companyID, userQuery, nil)
}

// ── Step 1: Embed query ───────────────────────────────────────────────────────

func embedQuery(ctx context.Context, query string) ([]float64, error) {
	client := openai.NewClient(option.WithAPIKey(LLM_KEY))

	resp, err := client.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Model: openai.EmbeddingModelTextEmbedding3Large,
		Input: openai.EmbeddingNewParamsInputUnion{
			OfString: openai.String(query),
		},
	})
	if err != nil {
		return nil, err
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}

	raw := resp.Data[0].Embedding
	vec := make([]float64, len(raw))
	for i, v := range raw {
		vec[i] = v
	}
	return vec, nil
}

// ── Step 2: Search Qdrant ────────────────────────────────────────────────────

func collectionName(companyID string) string {
	// Must match Python: company_<uuid with _ instead of ->
	return "company_" + strings.ReplaceAll(companyID, "-", "_")
}

func searchQdrant(ctx context.Context, companyID string, vector []float64) ([]RetrievedChunk, error) {
	collection := collectionName(companyID)
	url := fmt.Sprintf("%s/collections/%s/points/search", qdrantURL, collection)

	body := qdrantSearchRequest{
		Vector:         vector,
		Limit:          topK,
		WithPayload:    true,
		ScoreThreshold: minScore,
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if qdrantAPIKey != "" {
		req.Header.Set("api-key", qdrantAPIKey)
	}

	httpClient := &http.Client{Timeout: 10 * time.Second}
	res, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("qdrant request failed: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusNotFound {
		// Collection doesn't exist for this company yet
		return nil, ErrCollectionNotFound
	}

	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("qdrant error %d: %s", res.StatusCode, string(b))
	}

	var searchResp qdrantSearchResponse
	if err := json.NewDecoder(res.Body).Decode(&searchResp); err != nil {
		return nil, err
	}

	return parseChunks(searchResp.Result), nil
}

func parseChunks(points []qdrantPoint) []RetrievedChunk {
	chunks := make([]RetrievedChunk, 0, len(points))
	for _, p := range points {
		chunk := RetrievedChunk{
			Score: p.Score,
		}
		if v, ok := p.Payload["text"].(string); ok {
			chunk.Text = v
		}
		if v, ok := p.Payload["section_path"].(string); ok {
			chunk.SectionPath = v
		}
		if v, ok := p.Payload["intent"].(string); ok {
			chunk.Intent = v
		}
		if v, ok := p.Payload["source_type"].(string); ok {
			chunk.SourceType = v
		}
		if v, ok := p.Payload["file_name"].(string); ok {
			chunk.FileName = v
		}
		if v, ok := p.Payload["page_number"]; ok {
			chunk.PageNumber = v
		}
		// Extract any URL from the chunk text
		chunk.URI = extractURI(chunk.Text)

		chunks = append(chunks, chunk)
	}
	return chunks
}

// extractURI finds the first http/https URL in a text string.
func extractURI(text string) string {
	words := strings.Fields(text)
	for _, w := range words {
		w = strings.Trim(w, "(),[]")
		if strings.HasPrefix(w, "http://") || strings.HasPrefix(w, "https://") {
			return w
		}
	}
	return ""
}

// ── Step 3: Evaluate relevance ────────────────────────────────────────────────

func evaluateRelevance(chunks []RetrievedChunk) (relevant bool, hasData bool) {
	if len(chunks) == 0 {
		return false, false
	}

	// Check top chunk score
	topScore := chunks[0].Score
	if topScore < minScore {
		return false, false
	}

	// Check if we have meaningful text content
	totalText := 0
	for _, c := range chunks {
		totalText += len(strings.TrimSpace(c.Text))
	}

	relevant = topScore >= minScore
	hasData = totalText > 100 // at least some substantive content

	return relevant, hasData
}

// ── Step 4: Build dynamic prompt ─────────────────────────────────────────────

// buildGreetingPrompt returns a warm, professional greeting response.
func buildGreetingPrompt(userQuery string) string {
	return fmt.Sprintf(`You are a professional and friendly AI assistant representing this company.
The user has greeted you. Respond warmly and professionally.

LANGUAGE RULE: Respond in English by default. Only switch to another language if the user
has written a complete sentence (5+ words) clearly in that language. Never infer language
from a single word or short phrase.

Introduce yourself briefly as the company's AI assistant and invite them to ask any questions
about the company's products, services, or policies. Keep it short, welcoming, and natural.

User message: %s`, userQuery)
}

// buildNoCollectionPrompt handles the case where the company has no knowledge base yet.
func buildNoCollectionPrompt(userQuery string) string {
	return fmt.Sprintf(`You are a professional and empathetic AI assistant representing this company.
The user has asked a question but the company's knowledge base has not been set up yet.

LANGUAGE RULE: Respond in English by default. Only switch to another language if the user
has written a complete sentence (5+ words) clearly in that language. Never infer language
from a single word or short phrase — those are almost always English queries regardless of
how the word looks.

Apologise sincerely and let them know the knowledge base is not yet available.
Offer to connect them with a human agent who can assist them directly.
Ask warmly: "Would you like me to connect you with a human agent who can help you right away?"
Keep the tone warm, professional, and reassuring. Do not make up any information.

User question: %s`, userQuery)
}

func buildPrompt(userQuery string, chunks []RetrievedChunk, relevant, hasData bool) string {
	var sb strings.Builder

	// ── System persona ────────────────────────────────────────────────────────
	sb.WriteString(`You are a professional, knowledgeable, and helpful AI assistant representing this company. `)
	sb.WriteString(`Your role is to assist customers by providing accurate, clear, and courteous responses. `)
	sb.WriteString(`Always maintain a warm, formal, and professional tone. `)
	sb.WriteString(`Respond in the same language the user is writing in. `)
	sb.WriteString(`Do not fabricate information — only use the context provided below. `)
	sb.WriteString("If a URL or resource link is available and relevant, include it at the end of your response in a natural way, such as: 'For more details, you may visit: <url>'\n\n")

	// ── Handle irrelevant query ───────────────────────────────────────────────
	if !relevant {
		sb.WriteString("INSTRUCTION: The user's query does not appear to be related to this company's domain or the available knowledge base. ")
		sb.WriteString("Politely inform them that you can only assist with questions related to this company's products, services, and policies. ")
		sb.WriteString("Do not attempt to answer the query. Keep the response brief and friendly.")
		sb.WriteString("LANGUAGE RULE: Respond in English by default. Only switch to another language if the user has written a complete sentence (5+ words) clearly in that language. Never infer language from a single word or short phrase.")
		sb.WriteString(fmt.Sprintf("User query: %s", userQuery))
		return sb.String()
	}

	// ── Handle relevant but no/low data ──────────────────────────────────────
	if !hasData {
		sb.WriteString("INSTRUCTION: The user's query is relevant to this company, but the knowledge base does not contain sufficient information to give a complete answer. ")
		sb.WriteString("Acknowledge the question, share any small piece of relevant information you can from the context, then let the user know that you currently don't have detailed information on this topic. ")
		sb.WriteString("Offer to connect them with a human agent who can assist further. Ask: 'Would you like me to connect you with a human agent for this query?'")
		sb.WriteString("LANGUAGE RULE: Respond in English by default. Only switch to another language if the user has written a complete sentence (5+ words) clearly in that language. Never infer language from a single word or short phrase.")
		sb.WriteString(fmt.Sprintf("User query: %s", userQuery))
		return sb.String()
	}

	// ── Full RAG prompt with context ──────────────────────────────────────────
	sb.WriteString("INSTRUCTION: Use ONLY the context sections below to answer the user's question. ")
	sb.WriteString("Be thorough but concise. If the answer spans multiple context sections, synthesize them naturally. ")
	sb.WriteString("Do NOT mention 'context', 'chunks', or any internal system terms in your answer — just answer naturally as a company representative. ")
	sb.WriteString("If a URL is present in the context and is relevant to the answer, include it at the very end of your response like: 'For more information, you may visit: <url>'\n\n")

	// ── Inject context chunks ─────────────────────────────────────────────────
	sb.WriteString("--- CONTEXT ---\n")
	uris := []string{}

	for i, chunk := range chunks {
		if strings.TrimSpace(chunk.Text) == "" {
			continue
		}
		sb.WriteString(fmt.Sprintf("[%d] (relevance: %.0f%%, topic: %s)\n", i+1,
			chunk.Score*100, humanIntent(chunk.Intent)))
		sb.WriteString(chunk.Text)
		sb.WriteString("\n\n")

		if chunk.URI != "" {
			uris = appendUnique(uris, chunk.URI)
		}
	}
	sb.WriteString("--- END CONTEXT ---\n\n")

	// Pass URIs separately so model knows what's available
	if len(uris) > 0 {
		sb.WriteString("Available resource links (use only if directly relevant to the answer):\n")
		for _, u := range uris {
			sb.WriteString("- " + u + "\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("User question: %s\n", userQuery))
	sb.WriteString("\nProvide a helpful, professional answer based on the context above:")

	return sb.String()
}

// humanIntent maps internal intent tags to readable descriptions for the prompt.
func humanIntent(intent string) string {
	m := map[string]string{
		"policy_or_rule":      "policy/rules",
		"procedural":          "how-to/process",
		"pricing":             "pricing/costs",
		"contact_or_location": "contact/location",
		"product_or_service":  "product/service",
		"faq":                 "FAQ",
		"overview":            "overview",
		"summary":             "summary",
		"tabular_data":        "data table",
		"code_or_formula":     "technical",
		"informational":       "information",
		"navigation":          "navigation",
	}
	if v, ok := m[intent]; ok {
		return v
	}
	return intent
}

func appendUnique(slice []string, s string) []string {
	for _, v := range slice {
		if v == s {
			return slice
		}
	}
	return append(slice, s)
}

// cosineSimilarity is kept for optional local re-ranking.
func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// ── Step 5: Stream answer ─────────────────────────────────────────────────────

func streamAnswer(ctx context.Context, prompt string, onToken func(string)) error {
	client := openai.NewClient(option.WithAPIKey(LLM_KEY))

	stream := client.Responses.NewStreaming(ctx, responses.ResponseNewParams{
		Model: openai.ChatModelGPT4o,
		Input: responses.ResponseNewParamsInputUnion{
			OfString: openai.String(prompt),
		},
	})
	defer stream.Close()

	for stream.Next() {
		event := stream.Current()
		if event.Type == "response.output_text.delta" && onToken != nil {
			onToken(event.Delta)
		}
	}

	return stream.Err()
}
