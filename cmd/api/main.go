package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	chiadapter "github.com/awslabs/aws-lambda-go-api-proxy/chi"

	"github.com/aws/aws-sdk-go-v2/service/sesv2" // ✅ ADICIONAR
	httpAdapter "github.com/reangeline/backend_applywise/internal/adapters/inbound/http"
	"github.com/reangeline/backend_applywise/internal/adapters/outbound/ai/openai"
	"github.com/reangeline/backend_applywise/internal/adapters/outbound/auth/cognito"
	emailadapter "github.com/reangeline/backend_applywise/internal/adapters/outbound/email" // ✅ ADICIONAR
	fcmpublisher "github.com/reangeline/backend_applywise/internal/adapters/outbound/notification/fcm"
	"github.com/reangeline/backend_applywise/internal/adapters/outbound/payment/stripe"
	"github.com/reangeline/backend_applywise/internal/adapters/outbound/persistence/dynamodb"
	queue "github.com/reangeline/backend_applywise/internal/adapters/outbound/queue/sqs"
	s3storage "github.com/reangeline/backend_applywise/internal/adapters/outbound/storage/s3"
	appservice "github.com/reangeline/backend_applywise/internal/application/service"
	appconfig "github.com/reangeline/backend_applywise/pkg/config"
)

var chiLambda *chiadapter.ChiLambda

func init() {
	// Carrega configurações
	cfg := loadConfig()

	// Inicializa AWS SDK
	awsCfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Fatal("failed to load AWS config:", err)
	}

	// Inicializa adapters (outbound)
	dynamoClient := dynamodb.NewClient(awsCfg, cfg.DynamoDBTable)
	sesClient := sesv2.NewFromConfig(awsCfg)
	objectStorage := s3storage.NewObjectStorage(awsCfg, cfg.ResumesBucketName)

	userRepo := dynamodb.NewUserRepository(dynamoClient)
	subscriptionRepo := dynamodb.NewSubscriptionRepository(dynamoClient)
	resumeRepo := dynamodb.NewResumeRepository(dynamoClient)
	verificationRepo := dynamodb.NewVerificationRepository(dynamoClient)           // ✅ ADICIONAR
	creditTransactionRepo := dynamodb.NewCreditTransactionRepository(dynamoClient) // ✅ NOVO
	jobRepo := dynamodb.NewOptimizationJobRepository(dynamoClient)
	pipelineRepo := dynamodb.NewPipelineRepository(dynamoClient)
	contactRepo := dynamodb.NewContactRepository(dynamoClient)
	interviewRepo := dynamodb.NewInterviewRepository(dynamoClient)
	queuePublisher := queue.NewSQSPublisher(awsCfg, cfg.OptimizationQueueURL)
	fcmNotifier, err := fcmpublisher.NewPublisher(context.Background(), cfg.FirebaseCredentials, cfg.FirebaseProjectID, userRepo)
	if err != nil {
		log.Printf("warning: failed to init FCM notifier: %v", err)
	}

	emailService := emailadapter.NewEmailService(emailadapter.EmailConfig{
		Client:    sesClient,
		FromEmail: os.Getenv("SES_FROM_EMAIL"),
		FromName:  "ApplyWise",
		IsDev:     os.Getenv("ENVIRONMENT") == "dev",
	})

	cognitoClient := cognito.NewAuthProvider(awsCfg, cfg.CognitoUserPoolID, cfg.CognitoClientID)
	stripeClient := stripe.NewPaymentGateway(cfg.StripeSecretKey, cfg.WebAppBaseURL)
	aiClient := openai.NewAIService(cfg.OpenAIKey)

	// Inicializa services (application layer)
	userService := appservice.NewUserService(userRepo, resumeRepo, objectStorage, cognitoClient, subscriptionRepo, verificationRepo)

	authService := appservice.NewAuthService(
		cognitoClient,
		userRepo,
		subscriptionRepo,
		verificationRepo,
		emailService,
		resumeRepo,
	)

	subscriptionService := appservice.NewSubscriptionService(subscriptionRepo, stripeClient, userRepo, creditTransactionRepo, cfg.StripePricePremiumMonthly)
	paymentService := appservice.NewPaymentService(stripeClient, subscriptionRepo, cfg.StripeWebhookSecret, cfg.StripePricePremiumMonthly)
	resumeService := appservice.NewResumeOptimizerService(resumeRepo, aiClient, subscriptionRepo, creditTransactionRepo, queuePublisher, jobRepo, fcmNotifier)
	pipelineCoachService := appservice.NewPipelineCoachService(pipelineRepo, aiClient, subscriptionRepo, creditTransactionRepo)
	interviewPracticeService := appservice.NewInterviewPracticeService(pipelineRepo, interviewRepo, resumeRepo, aiClient, subscriptionRepo, creditTransactionRepo)
	applyAssistService := appservice.NewApplyAssistService(resumeRepo, subscriptionRepo, aiClient)

	revenueCatService := appservice.NewRevenueCatService(
		subscriptionRepo,
		userRepo,
		creditTransactionRepo,
	)

	// Inicializa router (inbound adapter)
	router := httpAdapter.NewRouter(
		authService,
		userService,
		subscriptionService,
		resumeService,
		paymentService,
		revenueCatService,
		pipelineRepo,
		contactRepo,
		userRepo,
		fcmNotifier,
		pipelineCoachService,
		interviewPracticeService,
		applyAssistService,
	)

	// Configura Lambda adapter
	chiLambda = chiadapter.New(router)
}

func Handler(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	return chiLambda.ProxyWithContext(ctx, req)
}

func main() {
	// Se está rodando localmente, usa servidor HTTP
	if os.Getenv("AWS_LAMBDA_FUNCTION_NAME") == "" {
		runLocalServer()
	} else {
		// Se está na Lambda, usa o handler
		lambda.Start(Handler)
	}
}

func runLocalServer() {
	cfg := loadConfig()

	// ... (mesmo código de init mas sem chiLambda)
	awsCfg, _ := config.LoadDefaultConfig(context.Background())
	dynamoClient := dynamodb.NewClient(awsCfg, cfg.DynamoDBTable)
	objectStorage := s3storage.NewObjectStorage(awsCfg, cfg.ResumesBucketName)

	userRepo := dynamodb.NewUserRepository(dynamoClient)
	subscriptionRepo := dynamodb.NewSubscriptionRepository(dynamoClient)
	resumeRepo := dynamodb.NewResumeRepository(dynamoClient)
	verificationRepo := dynamodb.NewVerificationRepository(dynamoClient)
	creditTransactionRepo := dynamodb.NewCreditTransactionRepository(dynamoClient)
	jobRepo := dynamodb.NewOptimizationJobRepository(dynamoClient)
	pipelineRepo := dynamodb.NewPipelineRepository(dynamoClient)
	contactRepo := dynamodb.NewContactRepository(dynamoClient)
	interviewRepo := dynamodb.NewInterviewRepository(dynamoClient)
	cognitoClient := cognito.NewAuthProvider(awsCfg, cfg.CognitoUserPoolID, cfg.CognitoClientID)
	stripeClient := stripe.NewPaymentGateway(cfg.StripeSecretKey, cfg.WebAppBaseURL)
	aiClient := openai.NewAIService(cfg.OpenAIKey)
	queuePublisher := queue.NewSQSPublisher(awsCfg, cfg.OptimizationQueueURL)
	fcmNotifier, err := fcmpublisher.NewPublisher(context.Background(), cfg.FirebaseCredentials, cfg.FirebaseProjectID, userRepo)
	if err != nil {
		log.Printf("warning: failed to init FCM notifier: %v", err)
	}

	sesClient := sesv2.NewFromConfig(awsCfg)

	emailService := emailadapter.NewEmailService(emailadapter.EmailConfig{
		Client:    sesClient,
		FromEmail: os.Getenv("SES_FROM_EMAIL"),
		FromName:  "ApplyWise",
		IsDev:     os.Getenv("ENVIRONMENT") == "dev",
	})

	userService := appservice.NewUserService(userRepo, resumeRepo, objectStorage, cognitoClient, subscriptionRepo, verificationRepo)
	authService := appservice.NewAuthService(cognitoClient, userRepo, subscriptionRepo, verificationRepo, emailService, resumeRepo)

	subscriptionService := appservice.NewSubscriptionService(subscriptionRepo, stripeClient, userRepo, creditTransactionRepo, cfg.StripePricePremiumMonthly)
	paymentService := appservice.NewPaymentService(stripeClient, subscriptionRepo, cfg.StripeWebhookSecret, cfg.StripePricePremiumMonthly)

	revenueCatService := appservice.NewRevenueCatService(
		subscriptionRepo,
		userRepo,
		creditTransactionRepo,
	)

	resumeService := appservice.NewResumeOptimizerService(resumeRepo, aiClient, subscriptionRepo, creditTransactionRepo, queuePublisher, jobRepo, fcmNotifier)
	pipelineCoachSvc := appservice.NewPipelineCoachService(pipelineRepo, aiClient, subscriptionRepo, creditTransactionRepo)
	interviewPracticeSvc := appservice.NewInterviewPracticeService(pipelineRepo, interviewRepo, resumeRepo, aiClient, subscriptionRepo, creditTransactionRepo)
	applyAssistSvc := appservice.NewApplyAssistService(resumeRepo, subscriptionRepo, aiClient)

	router := httpAdapter.NewRouter(
		authService,
		userService,
		subscriptionService,
		resumeService,
		paymentService,
		revenueCatService,
		pipelineRepo,
		contactRepo,
		userRepo,
		fcmNotifier,
		pipelineCoachSvc,
		interviewPracticeSvc,
		applyAssistSvc,
	)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Starting server on :%s", port)
	if err := http.ListenAndServe(":"+port, router); err != nil {
		log.Fatal(err)
	}
}

func loadConfig() *appconfig.Config {
	webAppBaseURL := os.Getenv("WEB_APP_BASE_URL")
	if webAppBaseURL == "" {
		webAppBaseURL = "http://localhost:3000"
	}

	return &appconfig.Config{
		DynamoDBTable:             os.Getenv("DYNAMODB_TABLE"),
		CognitoUserPoolID:         os.Getenv("COGNITO_USER_POOL_ID"),
		CognitoClientID:           os.Getenv("COGNITO_CLIENT_ID"),
		ResumesBucketName:         os.Getenv("RESUMES_BUCKET_NAME"),
		StripeSecretKey:           os.Getenv("STRIPE_SECRET_KEY"),
		StripeWebhookSecret:       os.Getenv("STRIPE_WEBHOOK_SECRET"),
		StripePricePremiumMonthly: os.Getenv("STRIPE_PRICE_PREMIUM_MONTHLY"),
		WebAppBaseURL:             webAppBaseURL,
		OpenAIKey:                 os.Getenv("OPENAI_API_KEY"),
		OptimizationQueueURL:      os.Getenv("OPTIMIZATION_QUEUE_URL"),
		FirebaseCredentials:       os.Getenv("FIREBASE_CREDENTIALS_FILE"),
		FirebaseProjectID:         os.Getenv("FIREBASE_PROJECT_ID"),
	}
}
