package dynamodb

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/reangeline/backend_applywise/internal/core/domain"
	"github.com/reangeline/backend_applywise/internal/core/ports/outbound"
)

type linkedInScanRepositoryImpl struct {
	client *Client
}

func NewLinkedInScanRepository(client *Client) outbound.LinkedInScanRepository {
	return &linkedInScanRepositoryImpl{client: client}
}

// Singleton por usuário — PK USER#<user_id>, SK fixo LINKEDINSCAN (sem histórico, spec 015).
const linkedInScanSK = "LINKEDINSCAN"

type LinkedInScanCheckItem struct {
	Label       string `dynamodbav:"Label"`
	Passed      bool   `dynamodbav:"Passed"`
	Explanation string `dynamodbav:"Explanation"`
}

type LinkedInScanSectionItem struct {
	Name   string                  `dynamodbav:"Name"`
	Checks []LinkedInScanCheckItem `dynamodbav:"Checks"`
}

type LinkedInScanItem struct {
	PK              string                    `dynamodbav:"PK"`
	SK              string                    `dynamodbav:"SK"`
	Type            string                    `dynamodbav:"Type"`
	ID              string                    `dynamodbav:"ID"`
	UserID          string                    `dynamodbav:"UserID"`
	Score           float64                   `dynamodbav:"Score"`
	Sections        []LinkedInScanSectionItem `dynamodbav:"Sections"`
	PredictedSkills []string                  `dynamodbav:"PredictedSkills"`
	Tips            []string                  `dynamodbav:"Tips"`
	CreatedAt       string                    `dynamodbav:"CreatedAt"`
	UpdatedAt       string                    `dynamodbav:"UpdatedAt"`
}

func (r *linkedInScanRepositoryImpl) Upsert(ctx context.Context, scan *domain.LinkedInScan) error {
	sections := make([]LinkedInScanSectionItem, 0, len(scan.Sections))
	for _, s := range scan.Sections {
		checks := make([]LinkedInScanCheckItem, 0, len(s.Checks))
		for _, c := range s.Checks {
			checks = append(checks, LinkedInScanCheckItem{
				Label:       c.Label,
				Passed:      c.Passed,
				Explanation: c.Explanation,
			})
		}
		sections = append(sections, LinkedInScanSectionItem{Name: s.Name, Checks: checks})
	}

	item := LinkedInScanItem{
		PK:              fmt.Sprintf("USER#%s", scan.UserID),
		SK:              linkedInScanSK,
		Type:            "LINKEDIN_SCAN",
		ID:              scan.ID,
		UserID:          scan.UserID,
		Score:           scan.Score,
		Sections:        sections,
		PredictedSkills: scan.PredictedSkills,
		Tips:            scan.Tips,
		CreatedAt:       scan.CreatedAt.Format(time.RFC3339),
		UpdatedAt:       scan.UpdatedAt.Format(time.RFC3339),
	}

	av, err := attributevalue.MarshalMap(item)
	if err != nil {
		return err
	}

	_, err = r.client.db.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(r.client.tableName),
		Item:      av,
	})

	return err
}

func (r *linkedInScanRepositoryImpl) GetByUserID(ctx context.Context, userID string) (*domain.LinkedInScan, error) {
	result, err := r.client.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(r.client.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", userID)},
			"SK": &types.AttributeValueMemberS{Value: linkedInScanSK},
		},
	})
	if err != nil {
		return nil, err
	}

	if result.Item == nil {
		return nil, domain.ErrLinkedInScanNotFound
	}

	var item LinkedInScanItem
	if err := attributevalue.UnmarshalMap(result.Item, &item); err != nil {
		return nil, err
	}

	return itemToLinkedInScan(&item)
}

func itemToLinkedInScan(item *LinkedInScanItem) (*domain.LinkedInScan, error) {
	createdAt, err := time.Parse(time.RFC3339, item.CreatedAt)
	if err != nil {
		return nil, err
	}
	updatedAt, err := time.Parse(time.RFC3339, item.UpdatedAt)
	if err != nil {
		return nil, err
	}

	sections := make([]domain.LinkedInScanSection, 0, len(item.Sections))
	for _, s := range item.Sections {
		checks := make([]domain.LinkedInScanCheck, 0, len(s.Checks))
		for _, c := range s.Checks {
			checks = append(checks, domain.LinkedInScanCheck{
				Label:       c.Label,
				Passed:      c.Passed,
				Explanation: c.Explanation,
			})
		}
		sections = append(sections, domain.LinkedInScanSection{Name: s.Name, Checks: checks})
	}

	return &domain.LinkedInScan{
		ID:              item.ID,
		UserID:          item.UserID,
		Score:           item.Score,
		Sections:        sections,
		PredictedSkills: item.PredictedSkills,
		Tips:            item.Tips,
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
	}, nil
}
