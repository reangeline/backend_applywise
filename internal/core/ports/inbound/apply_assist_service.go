package inbound

import "context"

// SuggestAnswerRequest pede uma sugestão de resposta pra uma pergunta de triagem de
// candidatura (ex: Easy Apply do LinkedIn), a partir de um currículo já salvo do usuário.
// Não está ligado a uma vaga do pipeline — acontece antes de a vaga existir no board, já
// que o registro no pipeline só ocorre depois que o usuário confirma o envio (spec 011).
type SuggestAnswerRequest struct {
	UserID         string
	ResumeID       string
	JobTitle       string
	CompanyName    string
	JobDescription string
	Question       string
}

// SuggestAnswerResult é a resposta sugerida pela IA.
type SuggestAnswerResult struct {
	SuggestedAnswer string `json:"suggested_answer"`
}

// ApplyAssistService assiste o preenchimento de candidaturas fora do produto (ex: extensão
// de navegador no LinkedIn). Feature exclusiva de assinantes Premium — sem consumo de
// crédito (spec 011).
type ApplyAssistService interface {
	SuggestAnswer(ctx context.Context, req SuggestAnswerRequest) (*SuggestAnswerResult, error)
}
