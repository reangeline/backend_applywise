variable "aws_region" {
  description = "AWS Region"
  type        = string
  default     = "us-east-1"
}

variable "environment" {
  description = "Environment name"
  type        = string
  default     = "dev"
}

variable "app_name" {
  description = "Application name"
  type        = string
  default     = "resume-optimizer"
}

variable "dynamodb_table_name" {
  description = "DynamoDB table name"
  type        = string
  default     = "resume-optimizer-dev"
}

variable "stripe_secret_key" {
  description = "Stripe Secret Key"
  type        = string
  sensitive   = true
}

variable "stripe_webhook_secret" {
  description = "Stripe Webhook Secret"
  type        = string
  sensitive   = true
}

variable "openai_api_key" {
  description = "OpenAI API Key"
  type        = string
  sensitive   = true
}

variable "stripe_price_premium_monthly" {
  description = "Stripe Price ID do plano Premium (US$19,99/mês) — spec 007"
  type        = string
}

variable "web_app_base_url" {
  description = "Base URL do web-app, usada nas URLs de sucesso/cancelamento do Stripe Checkout"
  type        = string
  default     = "https://hirefy-web-git-develop-reangelinehotmailcoms-projects.vercel.app"
}

variable "ses_from_email" {
  description = "Email to send from"
  type        = string
  default     = "contact@hirefy.careers"
}

variable "revenuecat_webhook_secret" {
  description = "RevenueCat webhook secret"
  type        = string
  default     = "" # Vazio por enquanto, vamos configurar depois
}

variable "revenuecat_api_key" {
  description = "RevenueCat API key (unused for now)"
  type        = string
  default     = ""
}

variable "cognito_domain_prefix" {
  description = "Cognito domain prefix (unused for now)"
  type        = string
  default     = ""
}

variable "optimization_queue_name" {
  description = "SQS queue name for async resume optimization"
  type        = string
  default     = "applywise-optimization-dev"
}

variable "firebase_credentials_file" {
  description = "Path to Firebase service account JSON inside the Lambda package or layer"
  type        = string
  default     = "/var/task/firebase_credentials.json"
}

variable "firebase_project_id" {
  description = "Firebase project ID (used to init FCM)"
  type        = string
  default     = "applywise-35cc7"
}