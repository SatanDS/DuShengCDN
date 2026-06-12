package model

import (
	"errors"
	"fmt"
	"gorm.io/gorm"
	"strings"
)

func ensureDefaultGitHubAuthSource(db *gorm.DB) error {
	if db == nil || !db.Migrator().HasTable(&AuthSource{}) || !db.Migrator().HasTable(&ExternalAccount{}) {
		return nil
	}

	var githubUserCount int64
	if db.Migrator().HasColumn(&User{}, "github_id") {
		if err := db.Model(&User{}).Where("github_id <> ''").Count(&githubUserCount).Error; err != nil {
			return fmt.Errorf("count legacy github users failed: %w", err)
		}
	}

	optionMap := map[string]string{}
	if db.Migrator().HasTable(&Option{}) {
		var options []Option
		if err := db.Find(&options).Error; err != nil {
			return fmt.Errorf("query options for github auth source migration failed: %w", err)
		}
		for _, option := range options {
			optionMap[option.Key] = option.Value
		}
	}

	clientID := strings.TrimSpace(optionMap["GitHubClientId"])
	clientSecret := strings.TrimSpace(optionMap["GitHubClientSecret"])
	enabled := optionMap["GitHubOAuthEnabled"] == "true" && clientID != "" && clientSecret != ""
	if githubUserCount == 0 && clientID == "" && clientSecret == "" {
		return nil
	}

	source := AuthSource{}
	err := db.Where("type = ? AND name = ?", AuthSourceTypeGitHub, "GitHub").First(&source).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		source = AuthSource{
			Name:         "GitHub",
			Type:         AuthSourceTypeGitHub,
			DisplayName:  "GitHub",
			IsActive:     enabled,
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Scopes:       "user:email",
		}
		if err := db.Create(&source).Error; err != nil {
			return fmt.Errorf("create default github auth source failed: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("query default github auth source failed: %w", err)
	} else {
		updates := map[string]any{}
		if source.ClientID == "" && clientID != "" {
			updates["client_id"] = clientID
		}
		if source.ClientSecret == "" && clientSecret != "" {
			updates["client_secret"] = clientSecret
		}
		if source.Scopes == "" {
			updates["scopes"] = "user:email"
		}
		if enabled && !source.IsActive {
			updates["is_active"] = true
		}
		if len(updates) > 0 {
			if err := db.Model(&source).Updates(updates).Error; err != nil {
				return fmt.Errorf("update default github auth source failed: %w", err)
			}
		}
	}

	if githubUserCount == 0 {
		return nil
	}

	var users []User
	if err := db.Select("id", "github_id", "username", "email").Where("github_id <> ''").Find(&users).Error; err != nil {
		return fmt.Errorf("query legacy github users failed: %w", err)
	}
	for _, user := range users {
		account := ExternalAccount{
			AuthSourceID:     source.ID,
			UserID:           user.Id,
			ExternalID:       user.GitHubId,
			ExternalUsername: user.GitHubId,
			Email:            user.Email,
		}
		if err := db.Where(ExternalAccount{
			AuthSourceID: source.ID,
			ExternalID:   user.GitHubId,
		}).FirstOrCreate(&account).Error; err != nil {
			return fmt.Errorf("migrate github external account for user %d failed: %w", user.Id, err)
		}
	}
	return nil
}
