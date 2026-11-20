package assist

// TaskImprove rewrites a request for a generator (a video, a picture or a song), written in any language, with more
// detail and in the same language, so the person sees it and can correct it before sending it. It keeps what was asked.
const TaskImprove Task = "improve"

// improveTask is the prompt of the task that improves a request.
func improveTask() taskSpec {
	return taskSpec{
		system: "You improve a request that someone will give to an AI that makes videos, pictures or songs. Rewrite it in the SAME " +
			"language as the request. Keep everything the person asked for: the subject, the style, any words in quotes exactly as written, " +
			"any length, and whether the song has a voice. Add concrete detail (setting, light, mood, style, tempo or instruments) " +
			"without adding new subjects or changing what it is. Reply with ONE paragraph of at most 60 words and nothing else: " +
			"no preamble, no quotes, no explanation.",
		user:      func(text string) string { return "Request: " + text },
		maxTokens: 140,
	}
}
