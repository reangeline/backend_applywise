package http

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/reangeline/backend_applywise/internal/adapters/inbound/http/handler"
	"github.com/reangeline/backend_applywise/internal/adapters/inbound/http/middleware"
	"github.com/reangeline/backend_applywise/internal/core/ports/inbound"
	"github.com/reangeline/backend_applywise/internal/core/ports/outbound"
)

func NewRouter(
	authService inbound.AuthService,
	userService inbound.UserService,
	subscriptionService inbound.SubscriptionService,
	resumeService inbound.ResumeOptimizerService,
	paymentService inbound.PaymentService,
	revenueCatService inbound.RevenueCatService,
	pipelineRepo outbound.PipelineRepository,
	contactRepo outbound.ContactRepository,
	userRepo outbound.UserRepository,
	notifier outbound.NotificationPublisher,
	pipelineCoachService inbound.PipelineCoachService,
	interviewPracticeService inbound.InterviewPracticeService,
	applyAssistService inbound.ApplyAssistService,
) *chi.Mux {
	r := chi.NewRouter()

	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)
	// Limit request bodies to 2 MB to prevent payload-flooding attacks.
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, 2<<20) // 2 MiB
			next.ServeHTTP(w, r)
		})
	})

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "//") {
				r.URL.Path = strings.ReplaceAll(r.URL.Path, "//", "/")
			}
			next.ServeHTTP(w, r)
		})
	})

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"https://*", "http://*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Revenuecat-Signature"}, // ✅ ADICIONAR header
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	authHandler := handler.NewAuthHandler(authService, userService)
	resumeHandler := handler.NewResumeHandler(resumeService)
	subscriptionHandler := handler.NewSubscriptionHandler(subscriptionService)
	webhookHandler := handler.NewWebhookHandler(paymentService)
	revenueCatWebhookHandler := handler.NewRevenueCatWebhookHandler(revenueCatService)
	userHandler := handler.NewUserHandler(userService)
	pipelineHandler := handler.NewPipelineHandler(pipelineRepo, contactRepo, userRepo, notifier, pipelineCoachService, interviewPracticeService)
	applyAssistHandler := handler.NewApplyAssistHandler(applyAssistService)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","service":"applywise-api"}`))
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok","version":"v1"}`))
		})

		// Públicas
		r.Post("/auth/signup", authHandler.SignUp)
		r.Post("/auth/signin", authHandler.SignIn)
		r.Post("/auth/social", authHandler.SocialSignIn)
		r.Post("/auth/refresh", authHandler.RefreshToken)
		r.Post("/auth/confirm", authHandler.ConfirmSignUp)
		r.Post("/auth/resend-code", authHandler.ResendCode)
		r.Post("/auth/forgot-password", authHandler.ForgotPassword)
		r.Post("/auth/confirm-forgot-password", authHandler.ConfirmForgotPassword)
		r.Post("/webhooks/stripe", webhookHandler.HandleStripeWebhook)
		r.Post("/webhooks/revenuecat", revenueCatWebhookHandler.HandleWebhook) // ✅ ADICIONAR
		r.Post("/resumes/parse-pdf", resumeHandler.ParsePDFResume)

		// Protegidas
		r.Group(func(r chi.Router) {
			r.Use(middleware.AuthMiddleware(authService, userService))

			r.Post("/resumes", resumeHandler.UploadResume)
			r.Post("/resumes/manual", resumeHandler.CreateManualResume)
			r.Get("/resumes", resumeHandler.ListResumes)
			r.Get("/resumes/{resumeID}", resumeHandler.GetResume)
			r.Put("/resumes/manual/{resumeID}", resumeHandler.UpdateManualResume)
			r.Post("/resumes/optimize", resumeHandler.OptimizeResume)
			r.Get("/resumes/optimized", resumeHandler.ListOptimizedResumes)
			r.Get("/resumes/optimized/{optimizedID}", resumeHandler.GetOptimizedResume)
			r.Put("/resumes/optimized/{optimizedID}", resumeHandler.UpdateOptimizedResume)
			r.Delete("/resumes/{resumeID}", resumeHandler.DeleteResume)

			r.Get("/resumes/optimize/jobs/{jobID}", resumeHandler.GetOptimizationJobStatus)

			// LinkedIn optimization
			r.Post("/resumes/linkedin/optimize", resumeHandler.OptimizeForLinkedIn)

			r.Get("/subscription", subscriptionHandler.GetSubscription)
			r.Post("/subscription", subscriptionHandler.CreateSubscription)
			r.Post("/subscription/checkout", subscriptionHandler.CreateCheckout)
			r.Delete("/subscription", subscriptionHandler.CancelSubscription)
			r.Get("/subscription/credits", subscriptionHandler.GetCredits)

			r.Get("/users/me", userHandler.GetMe)
			r.Patch("/users/me", userHandler.UpdateMe)
			r.Post("/users/me/fcm-token", userHandler.UpdateFCMToken)
			r.Delete("/users/me", userHandler.DeleteMe)

			// Pipeline (job application tracking)
			r.Get("/pipeline", pipelineHandler.ListJobs)
			r.Post("/pipeline", pipelineHandler.CreateJob)
			r.Get("/pipeline/{jobId}", pipelineHandler.GetJob)
			r.Put("/pipeline/{jobId}", pipelineHandler.UpdateJob)
			r.Delete("/pipeline/{jobId}", pipelineHandler.DeleteJob)
			r.Post("/pipeline/{jobId}/ghost", pipelineHandler.GhostJob)
			r.Post("/pipeline/{jobId}/interview", pipelineHandler.LogInterview)
			r.Post("/pipeline/{jobId}/followup", pipelineHandler.LogFollowUp)
			r.Post("/pipeline/{jobId}/coach", pipelineHandler.Coach)
			r.Get("/pipeline/analytics", pipelineHandler.GetPipelineAnalytics)
			r.Get("/pipeline/{jobId}/contacts", pipelineHandler.ListContacts)
			r.Post("/pipeline/{jobId}/contacts", pipelineHandler.AddContact)
			r.Delete("/pipeline/{jobId}/contacts/{contactId}", pipelineHandler.DeleteContact)
			// Prática de entrevista (spec 010) — "interview-practice" pra não colidir com
			// POST /pipeline/{jobId}/interview, que é o registro de uma entrevista agendada.
			r.Get("/pipeline/{jobId}/interview-practice", pipelineHandler.ListInterviewQuestions)
			r.Post("/pipeline/{jobId}/interview-practice/question", pipelineHandler.NextInterviewQuestion)
			r.Post("/pipeline/{jobId}/interview-practice/{questionId}/answer", pipelineHandler.SubmitInterviewAnswer)

			// Apply-assist (spec 011) — assiste preenchimento de candidatura fora do produto
			// (extensão de navegador). Não fica sob /pipeline/{jobId} porque acontece antes de
			// a vaga existir no board (só é registrada depois que o usuário confirma o envio).
			r.Post("/apply-assist/answer", applyAssistHandler.SuggestAnswer)
		})
	})

	return r
}
