// package llm

// import (
// 	"bytes"
// 	"context"
// 	"encoding/json"
// 	"fmt"
// 	"io"
// 	"math"
// 	"net/http"
// 	"os"
// 	"strings"
// 	"time"

// 	"github.com/openai/openai-go/v3"
// 	"github.com/openai/openai-go/v3/option"
// 	"github.com/openai/openai-go/v3/responses"
// )

// // ── Config ────────────────────────────────────────────────────────────────────

// var (
// 	qdrantURL      = getEnv("QDRANT_URL", "http://localhost:6333")
// 	qdrantAPIKey   = getEnv("QDRANT_API_KEY", "")
// 	embeddingModel = "text-embedding-3-large"
// 	topK           = 8    // how many chunks to retrieve
// 	minScore       = 0.35 // relevance threshold — below this = no useful match
// )

// // ErrCollectionNotFound is returned when the company has no Qdrant collection yet.
// var ErrCollectionNotFound = fmt.Errorf("knowledge base not found for this company")

// // greetingPhrases catches common greetings in any language so we respond warmly
// // instead of treating them as irrelevant queries.
// var greetingPhrases = []string{
// 	"hi", "hello", "hey", "hiya", "howdy",
// 	"good morning", "good afternoon", "good evening", "good night",
// 	"salam", "salaam", "assalamu alaikum", "assalamualaikum",
// 	"আস্সালামু আলাইকুম", "সালাম", "হ্যালো", "হ্যা", "হেলো",
// 	"নমস্কার", "নমস্তে", "কেমন আছেন", "কি খবর",
// 	"bonjour", "hola", "ciao", "مرحبا", "السلام عليكم",
// }

// func isGreeting(query string) bool {
// 	q := strings.ToLower(strings.TrimSpace(query))
// 	for _, g := range greetingPhrases {
// 		if q == g || strings.HasPrefix(q, g+" ") || strings.HasSuffix(q, " "+g) {
// 			return true
// 		}
// 	}
// 	return false
// }

// func getEnv(key, fallback string) string {
// 	if v := os.Getenv(key); v != "" {
// 		return v
// 	}
// 	return fallback
// }

// // ── Qdrant types ──────────────────────────────────────────────────────────────

// type qdrantSearchRequest struct {
// 	Vector         []float64 `json:"vector"`
// 	Limit          int       `json:"limit"`
// 	WithPayload    bool      `json:"with_payload"`
// 	ScoreThreshold float64   `json:"score_threshold"`
// }

// type qdrantPoint struct {
// 	ID      string                 `json:"id"`
// 	Score   float64                `json:"score"`
// 	Payload map[string]interface{} `json:"payload"`
// }

// type qdrantSearchResponse struct {
// 	Result []qdrantPoint `json:"result"`
// 	Status string        `json:"status"`
// }

// // ── RetrievedChunk ────────────────────────────────────────────────────────────

// type RetrievedChunk struct {
// 	Text        string
// 	Score       float64
// 	SectionPath string
// 	Intent      string
// 	SourceType  string
// 	FileName    string
// 	PageNumber  interface{}
// 	URI         string // extracted URL from text if present
// }

// // ── RAG Response ──────────────────────────────────────────────────────────────

// type RAGResult struct {
// 	Answer   string
// 	Chunks   []RetrievedChunk
// 	Relevant bool
// 	HasData  bool
// }

// // ── Public API ────────────────────────────────────────────────────────────────

// // RetrieveAndAnswer performs full RAG: embed → search → prompt → stream answer.
// // onToken is called for each streamed token. Returns the full answer when done.
// func RetrieveAndAnswer(
// 	ctx context.Context,
// 	companyID string,
// 	userQuery string,
// 	onToken func(token string),
// ) (RAGResult, error) {

// 	// 1. Embed the user query
// 	embedding, err := embedQuery(ctx, userQuery)
// 	if err != nil {
// 		return RAGResult{}, fmt.Errorf("embedding query: %w", err)
// 	}

// 	// 2. Detect greeting early — no need to hit Qdrant
// 	if isGreeting(userQuery) {
// 		prompt := buildGreetingPrompt(userQuery)
// 		var fullAnswer strings.Builder
// 		err = streamAnswer(ctx, prompt, func(token string) {
// 			fullAnswer.WriteString(token)
// 			if onToken != nil {
// 				onToken(token)
// 			}
// 		})
// 		if err != nil {
// 			return RAGResult{}, fmt.Errorf("streaming greeting: %w", err)
// 		}
// 		return RAGResult{Answer: fullAnswer.String(), Relevant: true, HasData: true}, nil
// 	}

// 	// 3. Search Qdrant for the company's collection
// 	chunks, err := searchQdrant(ctx, companyID, embedding)
// 	if err != nil {
// 		if err == ErrCollectionNotFound {
// 			prompt := buildNoCollectionPrompt(userQuery)
// 			var fullAnswer strings.Builder
// 			_ = streamAnswer(ctx, prompt, func(token string) {
// 				fullAnswer.WriteString(token)
// 				if onToken != nil {
// 					onToken(token)
// 				}
// 			})
// 			return RAGResult{
// 				Answer:   fullAnswer.String(),
// 				Relevant: false,
// 				HasData:  false,
// 			}, nil
// 		}
// 		return RAGResult{}, fmt.Errorf("qdrant search: %w", err)
// 	}

// 	// 4. Evaluate relevance
// 	relevant, hasData := evaluateRelevance(chunks)

// 	// 5. Build prompt based on relevance
// 	prompt := buildPrompt(userQuery, chunks, relevant, hasData)

// 	// 5. Stream response
// 	var fullAnswer strings.Builder
// 	err = streamAnswer(ctx, prompt, func(token string) {
// 		fullAnswer.WriteString(token)
// 		if onToken != nil {
// 			onToken(token)
// 		}
// 	})
// 	if err != nil {
// 		return RAGResult{}, fmt.Errorf("streaming answer: %w", err)
// 	}

// 	return RAGResult{
// 		Answer:   fullAnswer.String(),
// 		Chunks:   chunks,
// 		Relevant: relevant,
// 		HasData:  hasData,
// 	}, nil
// }

// // RetrieveAndAnswerSync is the non-streaming version. Returns the full answer.
// func RetrieveAndAnswerSync(
// 	ctx context.Context,
// 	companyID string,
// 	userQuery string,
// ) (RAGResult, error) {
// 	return RetrieveAndAnswer(ctx, companyID, userQuery, nil)
// }

// // ── Step 1: Embed query ───────────────────────────────────────────────────────

// func embedQuery(ctx context.Context, query string) ([]float64, error) {
// 	client := openai.NewClient(option.WithAPIKey(LLM_KEY))

// 	resp, err := client.Embeddings.New(ctx, openai.EmbeddingNewParams{
// 		Model: openai.EmbeddingModelTextEmbedding3Large,
// 		Input: openai.EmbeddingNewParamsInputUnion{
// 			OfString: openai.String(query),
// 		},
// 	})
// 	if err != nil {
// 		return nil, err
// 	}
// 	if len(resp.Data) == 0 {
// 		return nil, fmt.Errorf("no embedding returned")
// 	}

// 	raw := resp.Data[0].Embedding
// 	vec := make([]float64, len(raw))
// 	for i, v := range raw {
// 		vec[i] = v
// 	}
// 	return vec, nil
// }

// // ── Step 2: Search Qdrant ────────────────────────────────────────────────────

// func collectionName(companyID string) string {
// 	// Must match Python: company_<uuid with _ instead of ->
// 	return "company_" + strings.ReplaceAll(companyID, "-", "_")
// }

// func searchQdrant(ctx context.Context, companyID string, vector []float64) ([]RetrievedChunk, error) {
// 	collection := collectionName(companyID)
// 	url := fmt.Sprintf("%s/collections/%s/points/search", qdrantURL, collection)

// 	body := qdrantSearchRequest{
// 		Vector:         vector,
// 		Limit:          topK,
// 		WithPayload:    true,
// 		ScoreThreshold: minScore,
// 	}

// 	bodyBytes, err := json.Marshal(body)
// 	if err != nil {
// 		return nil, err
// 	}

// 	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
// 	if err != nil {
// 		return nil, err
// 	}
// 	req.Header.Set("Content-Type", "application/json")
// 	if qdrantAPIKey != "" {
// 		req.Header.Set("api-key", qdrantAPIKey)
// 	}

// 	httpClient := &http.Client{Timeout: 10 * time.Second}
// 	res, err := httpClient.Do(req)
// 	if err != nil {
// 		return nil, fmt.Errorf("qdrant request failed: %w", err)
// 	}
// 	defer res.Body.Close()

// 	if res.StatusCode == http.StatusNotFound {
// 		// Collection doesn't exist for this company yet
// 		return nil, ErrCollectionNotFound
// 	}

// 	if res.StatusCode != http.StatusOK {
// 		b, _ := io.ReadAll(res.Body)
// 		return nil, fmt.Errorf("qdrant error %d: %s", res.StatusCode, string(b))
// 	}

// 	var searchResp qdrantSearchResponse
// 	if err := json.NewDecoder(res.Body).Decode(&searchResp); err != nil {
// 		return nil, err
// 	}

// 	return parseChunks(searchResp.Result), nil
// }

// func parseChunks(points []qdrantPoint) []RetrievedChunk {
// 	chunks := make([]RetrievedChunk, 0, len(points))
// 	for _, p := range points {
// 		chunk := RetrievedChunk{
// 			Score: p.Score,
// 		}
// 		if v, ok := p.Payload["text"].(string); ok {
// 			chunk.Text = v
// 		}
// 		if v, ok := p.Payload["section_path"].(string); ok {
// 			chunk.SectionPath = v
// 		}
// 		if v, ok := p.Payload["intent"].(string); ok {
// 			chunk.Intent = v
// 		}
// 		if v, ok := p.Payload["source_type"].(string); ok {
// 			chunk.SourceType = v
// 		}
// 		if v, ok := p.Payload["file_name"].(string); ok {
// 			chunk.FileName = v
// 		}
// 		if v, ok := p.Payload["page_number"]; ok {
// 			chunk.PageNumber = v
// 		}
// 		// Extract any URL from the chunk text
// 		chunk.URI = extractURI(chunk.Text)

// 		chunks = append(chunks, chunk)
// 	}
// 	return chunks
// }

// // extractURI finds the first http/https URL in a text string.
// func extractURI(text string) string {
// 	words := strings.Fields(text)
// 	for _, w := range words {
// 		w = strings.Trim(w, "(),[]")
// 		if strings.HasPrefix(w, "http://") || strings.HasPrefix(w, "https://") {
// 			return w
// 		}
// 	}
// 	return ""
// }

// // ── Step 3: Evaluate relevance ────────────────────────────────────────────────

// func evaluateRelevance(chunks []RetrievedChunk) (relevant bool, hasData bool) {
// 	if len(chunks) == 0 {
// 		return false, false
// 	}

// 	// Check top chunk score
// 	topScore := chunks[0].Score
// 	if topScore < minScore {
// 		return false, false
// 	}

// 	// Check if we have meaningful text content
// 	totalText := 0
// 	for _, c := range chunks {
// 		totalText += len(strings.TrimSpace(c.Text))
// 	}

// 	relevant = topScore >= minScore
// 	hasData = totalText > 100 // at least some substantive content

// 	return relevant, hasData
// }

// // ── Step 4: Build dynamic prompt ─────────────────────────────────────────────

// // buildGreetingPrompt returns a warm, professional greeting response.
// func buildGreetingPrompt(userQuery string) string {
// 	return fmt.Sprintf(`You are a professional and friendly AI assistant representing this company.
// The user has greeted you. Respond warmly and professionally.

// LANGUAGE RULE: Respond in English by default. Only switch to another language if the user
// has written a complete sentence (5+ words) clearly in that language. Never infer language
// from a single word or short phrase.

// Introduce yourself briefly as the company's AI assistant and invite them to ask any questions
// about the company's products, services, or policies. Keep it short, welcoming, and natural.

// User message: %s`, userQuery)
// }

// // buildNoCollectionPrompt handles the case where the company has no knowledge base yet.
// func buildNoCollectionPrompt(userQuery string) string {
// 	return fmt.Sprintf(`You are a professional and empathetic AI assistant representing this company.
// The user has asked a question but the company's knowledge base has not been set up yet.

// LANGUAGE RULE: Respond in English by default. Only switch to another language if the user
// has written a complete sentence (5+ words) clearly in that language. Never infer language
// from a single word or short phrase — those are almost always English queries regardless of
// how the word looks.

// Apologise sincerely and let them know the knowledge base is not yet available.
// Offer to connect them with a human agent who can assist them directly.
// Ask warmly: "Would you like me to connect you with a human agent who can help you right away?"
// Keep the tone warm, professional, and reassuring. Do not make up any information.

// User question: %s`, userQuery)
// }

// func buildPrompt(userQuery string, chunks []RetrievedChunk, relevant, hasData bool) string {
// 	var sb strings.Builder

// 	// ── System persona ────────────────────────────────────────────────────────
// 	sb.WriteString(`You are a professional, knowledgeable, and helpful AI assistant representing this company. `)
// 	sb.WriteString(`Your role is to assist customers by providing accurate, clear, and courteous responses. `)
// 	sb.WriteString(`Always maintain a warm, formal, and professional tone. `)
// 	sb.WriteString(`Respond in the same language the user is writing in. `)
// 	sb.WriteString(`Do not fabricate information — only use the context provided below. `)
// 	sb.WriteString("If a URL or resource link is available and relevant, include it at the end of your response in a natural way, such as: 'For more details, you may visit: <url>'\n\n")

// 	// ── Handle irrelevant query ───────────────────────────────────────────────
// 	if !relevant {
// 		sb.WriteString("INSTRUCTION: The user's query does not appear to be related to this company's domain or the available knowledge base. ")
// 		sb.WriteString("Politely inform them that you can only assist with questions related to this company's products, services, and policies. ")
// 		sb.WriteString("Do not attempt to answer the query. Keep the response brief and friendly.")
// 		sb.WriteString("LANGUAGE RULE: Respond in English by default. Only switch to another language if the user has written a complete sentence (5+ words) clearly in that language. Never infer language from a single word or short phrase.")
// 		sb.WriteString(fmt.Sprintf("User query: %s", userQuery))
// 		return sb.String()
// 	}

// 	// ── Handle relevant but no/low data ──────────────────────────────────────
// 	if !hasData {
// 		sb.WriteString("INSTRUCTION: The user's query is relevant to this company, but the knowledge base does not contain sufficient information to give a complete answer. ")
// 		sb.WriteString("Acknowledge the question, share any small piece of relevant information you can from the context, then let the user know that you currently don't have detailed information on this topic. ")
// 		sb.WriteString("Offer to connect them with a human agent who can assist further. Ask: 'Would you like me to connect you with a human agent for this query?'")
// 		sb.WriteString("LANGUAGE RULE: Respond in English by default. Only switch to another language if the user has written a complete sentence (5+ words) clearly in that language. Never infer language from a single word or short phrase.")
// 		sb.WriteString(fmt.Sprintf("User query: %s", userQuery))
// 		return sb.String()
// 	}

// 	// ── Full RAG prompt with context ──────────────────────────────────────────
// 	sb.WriteString("INSTRUCTION: Use ONLY the context sections below to answer the user's question. ")
// 	sb.WriteString("Be thorough but concise. If the answer spans multiple context sections, synthesize them naturally. ")
// 	sb.WriteString("Do NOT mention 'context', 'chunks', or any internal system terms in your answer — just answer naturally as a company representative. ")
// 	sb.WriteString("If a URL is present in the context and is relevant to the answer, include it at the very end of your response like: 'For more information, you may visit: <url>'\n\n")

// 	// ── Inject context chunks ─────────────────────────────────────────────────
// 	sb.WriteString("--- CONTEXT ---\n")
// 	uris := []string{}

// 	for i, chunk := range chunks {
// 		if strings.TrimSpace(chunk.Text) == "" {
// 			continue
// 		}
// 		sb.WriteString(fmt.Sprintf("[%d] (relevance: %.0f%%, topic: %s)\n", i+1,
// 			chunk.Score*100, humanIntent(chunk.Intent)))
// 		sb.WriteString(chunk.Text)
// 		sb.WriteString("\n\n")

// 		if chunk.URI != "" {
// 			uris = appendUnique(uris, chunk.URI)
// 		}
// 	}
// 	sb.WriteString("--- END CONTEXT ---\n\n")

// 	// Pass URIs separately so model knows what's available
// 	if len(uris) > 0 {
// 		sb.WriteString("Available resource links (use only if directly relevant to the answer):\n")
// 		for _, u := range uris {
// 			sb.WriteString("- " + u + "\n")
// 		}
// 		sb.WriteString("\n")
// 	}

// 	sb.WriteString(fmt.Sprintf("User question: %s\n", userQuery))
// 	sb.WriteString("\nProvide a helpful, professional answer based on the context above:")

// 	return sb.String()
// }

// // humanIntent maps internal intent tags to readable descriptions for the prompt.
// func humanIntent(intent string) string {
// 	m := map[string]string{
// 		"policy_or_rule":      "policy/rules",
// 		"procedural":          "how-to/process",
// 		"pricing":             "pricing/costs",
// 		"contact_or_location": "contact/location",
// 		"product_or_service":  "product/service",
// 		"faq":                 "FAQ",
// 		"overview":            "overview",
// 		"summary":             "summary",
// 		"tabular_data":        "data table",
// 		"code_or_formula":     "technical",
// 		"informational":       "information",
// 		"navigation":          "navigation",
// 	}
// 	if v, ok := m[intent]; ok {
// 		return v
// 	}
// 	return intent
// }

// func appendUnique(slice []string, s string) []string {
// 	for _, v := range slice {
// 		if v == s {
// 			return slice
// 		}
// 	}
// 	return append(slice, s)
// }

// // cosineSimilarity is kept for optional local re-ranking.
// func cosineSimilarity(a, b []float64) float64 {
// 	if len(a) != len(b) {
// 		return 0
// 	}
// 	var dot, normA, normB float64
// 	for i := range a {
// 		dot += a[i] * b[i]
// 		normA += a[i] * a[i]
// 		normB += b[i] * b[i]
// 	}
// 	if normA == 0 || normB == 0 {
// 		return 0
// 	}
// 	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
// }

// // ── Step 5: Stream answer ─────────────────────────────────────────────────────

// func streamAnswer(ctx context.Context, prompt string, onToken func(string)) error {
// 	client := openai.NewClient(option.WithAPIKey(LLM_KEY))

// 	stream := client.Responses.NewStreaming(ctx, responses.ResponseNewParams{
// 		Model: openai.ChatModelGPT4o,
// 		Input: responses.ResponseNewParamsInputUnion{
// 			OfString: openai.String(prompt),
// 		},
// 	})
// 	defer stream.Close()

// 	for stream.Next() {
// 		event := stream.Current()
// 		if event.Type == "response.output_text.delta" && onToken != nil {
// 			onToken(event.Delta)
// 		}
// 	}

// 	return stream.Err()
// }

// --------------------------v2--------------------------------
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
	qdrantURL    = getEnv("QDRANT_URL", "http://localhost:6333")
	qdrantAPIKey = getEnv("QDRANT_API_KEY", "")
	topK         = 10   // retrieve more candidates for better coverage
	minScore     = 0.30 // slightly lower threshold — let the prompt handle relevance
)

// ErrCollectionNotFound is returned when the company has no Qdrant collection yet.
var ErrCollectionNotFound = fmt.Errorf("knowledge base not found for this company")

// ── Greeting detection ────────────────────────────────────────────────────────

var greetingPhrases = []string{
	// English
	"hi", "hello", "hey", "hiya", "howdy", "greetings", "sup", "what's up", "whats up",
	"good morning", "good afternoon", "good evening", "good night", "good day",
	// Bengali
	"salam", "salaam", "assalamu alaikum", "assalamualaikum", "walaikum assalam",
	"আস্সালামু আলাইকুম", "সালাম", "হ্যালো", "হ্যা", "হেলো",
	"নমস্কার", "নমস্তে", "কেমন আছেন", "কি খবর", "ভালো আছেন",
	// Other languages
	"bonjour", "hola", "ciao", "مرحبا", "السلام عليكم", "shalom", "namaste",
	"konnichiwa", "ni hao", "merhaba",
}

// smallTalk catches questions that are conversational but not company-related
var smallTalkPhrases = []string{
	"how are you", "how r u", "how are you doing", "what are you", "who are you",
	"are you a bot", "are you ai", "are you human", "are you real",
	"what can you do", "can you help me", "help me", "i need help",
	"thank you", "thanks", "thank u", "thx", "ty",
	"ok", "okay", "alright", "cool", "great", "nice", "awesome",
	"bye", "goodbye", "see you", "take care",
}

func isGreeting(query string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	for _, g := range greetingPhrases {
		if q == g || strings.HasPrefix(q, g+" ") || strings.HasSuffix(q, " "+g) ||
			strings.HasPrefix(q, g+"!") || strings.HasPrefix(q, g+",") {
			return true
		}
	}
	return false
}

func isSmallTalk(query string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	for _, s := range smallTalkPhrases {
		if q == s || strings.Contains(q, s) {
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
	SourceURL   string
	PageTitle   string
	FileName    string
	PageNumber  interface{}
	Domain      string
	Language    string
}

// ── CompanyProfile — inferred from chunk metadata ─────────────────────────────

type CompanyProfile struct {
	Domain      string
	PageTitle   string
	Language    string
	Categories  []string // unique intents found in chunks
	HasProducts bool
	HasPricing  bool
	HasPolicy   bool
	HasContact  bool
}

func inferCompanyProfile(chunks []RetrievedChunk) CompanyProfile {
	profile := CompanyProfile{}
	seen := map[string]bool{}

	for _, c := range chunks {
		if profile.Domain == "" && c.Domain != "" {
			profile.Domain = c.Domain
		}
		if profile.Language == "" && c.Language != "" && c.Language != "en" {
			profile.Language = c.Language
		}
		if !seen[c.Intent] && c.Intent != "" {
			profile.Categories = append(profile.Categories, c.Intent)
			seen[c.Intent] = true
		}
		switch c.Intent {
		case "product_or_service":
			profile.HasProducts = true
		case "pricing":
			profile.HasPricing = true
		case "policy_or_rule":
			profile.HasPolicy = true
		case "contact_or_location":
			profile.HasContact = true
		}
	}

	return profile
}

// ── RAG Response ──────────────────────────────────────────────────────────────

type RAGResult struct {
	Answer   string
	Chunks   []RetrievedChunk
	Relevant bool
	HasData  bool
}

// ── Public API ────────────────────────────────────────────────────────────────

func RetrieveAndAnswer(
	ctx context.Context,
	companyID string,
	userQuery string,
	onToken func(token string),
) (RAGResult, error) {

	// 1. Handle greetings immediately — no Qdrant needed
	if isGreeting(userQuery) {
		// Still search Qdrant to get company profile for a personalised greeting
		embedding, _ := embedQuery(ctx, "company overview products services")
		chunks, _ := searchQdrant(ctx, companyID, embedding)
		profile := inferCompanyProfile(chunks)

		prompt := buildGreetingPrompt(userQuery, profile)
		var fullAnswer strings.Builder
		err := streamAnswer(ctx, prompt, func(token string) {
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

	// 2. Handle small talk
	if isSmallTalk(userQuery) {
		embedding, _ := embedQuery(ctx, "company overview products services")
		chunks, _ := searchQdrant(ctx, companyID, embedding)
		profile := inferCompanyProfile(chunks)

		prompt := buildSmallTalkPrompt(userQuery, profile)
		var fullAnswer strings.Builder
		err := streamAnswer(ctx, prompt, func(token string) {
			fullAnswer.WriteString(token)
			if onToken != nil {
				onToken(token)
			}
		})
		if err != nil {
			return RAGResult{}, fmt.Errorf("streaming small talk: %w", err)
		}
		return RAGResult{Answer: fullAnswer.String(), Relevant: true, HasData: true}, nil
	}

	// 3. Embed the actual user query
	embedding, err := embedQuery(ctx, userQuery)
	if err != nil {
		return RAGResult{}, fmt.Errorf("embedding query: %w", err)
	}

	// 4. Search Qdrant
	chunks, err := searchQdrant(ctx, companyID, embedding)
	if err != nil {
		if err == ErrCollectionNotFound {
			prompt := buildNoKnowledgeBasePrompt(userQuery)
			var fullAnswer strings.Builder
			_ = streamAnswer(ctx, prompt, func(token string) {
				fullAnswer.WriteString(token)
				if onToken != nil {
					onToken(token)
				}
			})
			return RAGResult{Answer: fullAnswer.String(), Relevant: false, HasData: false}, nil
		}
		return RAGResult{}, fmt.Errorf("qdrant search: %w", err)
	}

	// 5. Infer company profile from retrieved chunks
	profile := inferCompanyProfile(chunks)

	// 6. Evaluate relevance
	relevant, hasData := evaluateRelevance(chunks)

	// 7. Build prompt and stream
	prompt := buildPrompt(userQuery, chunks, profile, relevant, hasData)

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

func RetrieveAndAnswerSync(ctx context.Context, companyID string, userQuery string) (RAGResult, error) {
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
	// Matches Python ingester: company_<uuid with - replaced by _>
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
		chunk := RetrievedChunk{Score: p.Score}
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
		if v, ok := p.Payload["source_url"].(string); ok {
			chunk.SourceURL = v
		}
		if v, ok := p.Payload["page_title"].(string); ok {
			chunk.PageTitle = v
		}
		if v, ok := p.Payload["domain"].(string); ok {
			chunk.Domain = v
		}
		if v, ok := p.Payload["language"].(string); ok {
			chunk.Language = v
		}
		if v, ok := p.Payload["file_name"].(string); ok {
			chunk.FileName = v
		}
		if v, ok := p.Payload["page_number"]; ok {
			chunk.PageNumber = v
		}
		chunks = append(chunks, chunk)
	}
	return chunks
}

// ── Step 3: Evaluate relevance ────────────────────────────────────────────────

func evaluateRelevance(chunks []RetrievedChunk) (relevant bool, hasData bool) {
	if len(chunks) == 0 {
		return false, false
	}

	topScore := chunks[0].Score
	totalText := 0
	for _, c := range chunks {
		totalText += len(strings.TrimSpace(c.Text))
	}

	relevant = topScore >= minScore
	hasData = totalText > 150
	return relevant, hasData
}

// ── Step 4: Prompt builders ───────────────────────────────────────────────────

// systemPersona builds a dynamic company persona based on the inferred profile.
func systemPersona(profile CompanyProfile) string {
	var sb strings.Builder

	sb.WriteString("You are the official AI assistant representing this company")
	if profile.Domain != "" {
		sb.WriteString(fmt.Sprintf(" (%s)", profile.Domain))
	}
	sb.WriteString(".\n\n")

	sb.WriteString("YOUR ROLE:\n")
	sb.WriteString("- You speak AS the company, not about it. You are the company's voice.\n")
	sb.WriteString("- You are warm, professional, knowledgeable, and genuinely helpful.\n")
	sb.WriteString("- You feel like a human customer service representative who deeply knows the company.\n")
	sb.WriteString("- You never say 'according to our data' or 'based on the context' — just answer naturally.\n")
	sb.WriteString("- You never expose internal system terms like 'chunks', 'vectors', 'RAG', or 'knowledge base'.\n\n")

	sb.WriteString("LANGUAGE RULES:\n")
	sb.WriteString("- Detect the language of the user's message and respond in the SAME language.\n")
	sb.WriteString("- If the user writes in Bengali, respond in Bengali. If English, respond in English.\n")
	sb.WriteString("- Never mix languages unless the user does.\n\n")

	sb.WriteString("TONE:\n")
	sb.WriteString("- Warm and approachable, never robotic.\n")
	sb.WriteString("- Confident and accurate — if you know it, say it clearly.\n")
	sb.WriteString("- Concise but complete — don't pad with filler words.\n\n")

	return sb.String()
}

func buildGreetingPrompt(userQuery string, profile CompanyProfile) string {
	var sb strings.Builder
	sb.WriteString(systemPersona(profile))

	sb.WriteString("SITUATION: The customer has greeted you.\n\n")
	sb.WriteString("INSTRUCTIONS:\n")
	sb.WriteString("- Greet them back warmly and naturally — like a friendly company rep would.\n")
	sb.WriteString("- Briefly mention what you can help with based on what the company offers.\n")
	sb.WriteString("- Keep it short (2-3 sentences max). Don't be overly formal.\n")
	sb.WriteString("- Make it feel human, not like a chatbot auto-response.\n\n")

	if profile.Domain != "" {
		sb.WriteString(fmt.Sprintf("Company domain: %s\n", profile.Domain))
	}
	if profile.HasProducts {
		sb.WriteString("The company offers products/services you can ask about.\n")
	}

	sb.WriteString(fmt.Sprintf("\nCustomer message: %s\n", userQuery))
	sb.WriteString("\nRespond naturally as the company's representative:")
	return sb.String()
}

func buildSmallTalkPrompt(userQuery string, profile CompanyProfile) string {
	var sb strings.Builder
	sb.WriteString(systemPersona(profile))

	sb.WriteString("SITUATION: The customer is making small talk or asking a general conversational question.\n\n")
	sb.WriteString("INSTRUCTIONS:\n")
	sb.WriteString("- Respond in a friendly, natural way — like a real person would.\n")
	sb.WriteString("- Keep it brief and light.\n")
	sb.WriteString("- Gently steer the conversation toward how you can help them with the company's offerings.\n")
	sb.WriteString("- Don't lecture them or be overly promotional.\n\n")

	sb.WriteString(fmt.Sprintf("Customer message: %s\n", userQuery))
	sb.WriteString("\nRespond naturally:")
	return sb.String()
}

func buildNoKnowledgeBasePrompt(userQuery string) string {
	return fmt.Sprintf(`You are a professional and empathetic company AI assistant.

The company's knowledge base has not been set up yet, so you cannot answer specific questions.

INSTRUCTIONS:
- Apologise briefly and sincerely — one sentence.
- Let the customer know they can reach a human agent for help.
- Ask: "Would you like me to connect you with one of our team members?"
- Keep it warm and reassuring. Do not make up any information.

Detect and respond in the same language as the customer.

Customer question: %s

Respond naturally:`, userQuery)
}

func buildPrompt(userQuery string, chunks []RetrievedChunk, profile CompanyProfile, relevant, hasData bool) string {
	var sb strings.Builder

	sb.WriteString(systemPersona(profile))

	// ── Irrelevant query ──────────────────────────────────────────────────────
	if !relevant {
		sb.WriteString("SITUATION: The customer asked something unrelated to this company.\n\n")
		sb.WriteString("INSTRUCTIONS:\n")
		sb.WriteString("- Politely let them know you can only help with questions about this company.\n")
		sb.WriteString("- Don't be dismissive — be warm and offer to help with something you CAN answer.\n")
		sb.WriteString("- Give one example of what you CAN help with, based on the company's focus.\n")
		sb.WriteString("- Keep it to 2-3 sentences.\n\n")
		sb.WriteString(fmt.Sprintf("Customer question: %s\n", userQuery))
		sb.WriteString("\nRespond naturally:")
		return sb.String()
	}

	// ── Relevant but insufficient data ────────────────────────────────────────
	if !hasData {
		sb.WriteString("SITUATION: The customer asked something relevant, but we don't have enough detail to fully answer.\n\n")
		sb.WriteString("INSTRUCTIONS:\n")
		sb.WriteString("- Share any relevant information you do have from the context below.\n")
		sb.WriteString("- Be honest that you don't have complete details on this specific topic.\n")
		sb.WriteString("- Offer to connect them with a team member who can help further.\n")
		sb.WriteString("- Ask: 'Would you like me to connect you with one of our team members for more details?'\n\n")

		if len(chunks) > 0 {
			sb.WriteString("PARTIAL CONTEXT:\n")
			for _, c := range chunks {
				if strings.TrimSpace(c.Text) != "" {
					sb.WriteString(c.Text + "\n\n")
				}
			}
		}

		sb.WriteString(fmt.Sprintf("Customer question: %s\n", userQuery))
		sb.WriteString("\nRespond naturally:")
		return sb.String()
	}

	// ── Full answer with context ──────────────────────────────────────────────
	sb.WriteString("SITUATION: The customer has asked a question you can fully answer from the company's information.\n\n")
	sb.WriteString("INSTRUCTIONS:\n")
	sb.WriteString("- Answer naturally and confidently, as if you personally know the answer.\n")
	sb.WriteString("- Synthesize information from multiple context sections if needed — don't list them separately.\n")
	sb.WriteString("- Include relevant URLs or links at the END only if directly useful (format: 'You can find more at: <url>').\n")
	sb.WriteString("- If the question has multiple parts, address each one.\n")
	sb.WriteString("- Do NOT start with 'Based on...' or 'According to...' — just answer.\n")
	sb.WriteString("- Do NOT mention 'context', 'data', 'knowledge base', or any internal terms.\n\n")

	// ── Context injection ─────────────────────────────────────────────────────
	sb.WriteString("--- COMPANY INFORMATION ---\n")
	urls := []string{}

	for _, chunk := range chunks {
		text := strings.TrimSpace(chunk.Text)
		if text == "" {
			continue
		}
		// Add section context if available
		if chunk.SectionPath != "" {
			sb.WriteString(fmt.Sprintf("[%s]\n", chunk.SectionPath))
		}
		sb.WriteString(text)
		sb.WriteString("\n\n")

		// Collect unique source URLs
		if chunk.SourceURL != "" {
			urls = appendUnique(urls, chunk.SourceURL)
		}
	}
	sb.WriteString("--- END ---\n\n")

	if len(urls) > 0 {
		sb.WriteString("Available page links (include only if directly relevant):\n")
		for _, u := range urls {
			sb.WriteString("- " + u + "\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("Customer question: %s\n", userQuery))
	sb.WriteString("\nAnswer naturally and helpfully:")

	return sb.String()
}

// ── Helpers ───────────────────────────────────────────────────────────────────

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
