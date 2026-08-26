// setting.go
package dto

type SettingDto struct {
	AllowRegistration              *bool                  `json:"allowRegistration"`
	AutoApproveRegistration        *bool                  `json:"autoApproveRegistration"`
	FrontendSettings               map[string]interface{} `json:"frontendSettings"`
	SmtpEnabled                    *bool                  `json:"smtpEnabled"`
	SmtpDriver                     *string                `json:"smtpDriver"`
	SmtpHost                       *string                `json:"smtpHost"`
	SmtpPort                       *int                   `json:"smtpPort"`
	SmtpUser                       *string                `json:"smtpUser"`
	SmtpPass                       *string                `json:"smtpPass"`
	ClearSmtpUser                  *bool                  `json:"clearSmtpUser"`
	ClearSmtpPass                  *bool                  `json:"clearSmtpPass"`
	SmtpFrom                       *string                `json:"smtpFrom"`
	SmtpEncryption                 *string                `json:"smtpEncryption"`
	SmtpTLS                        *bool                  `json:"smtpTLS"`
	StorageEnabled                 *bool                  `json:"storageEnabled"`
	StorageConfig                  map[string]interface{} `json:"storageConfig"`
	AttachmentStorageEnabled       *bool                  `json:"attachmentStorageEnabled"`
	AttachmentStorageConfig        map[string]interface{} `json:"attachmentStorageConfig"`
	RecycleBinRetentionDays        *int                   `json:"recycleBinRetentionDays"`
	CommentRecycleBinRetentionDays *int                   `json:"commentRecycleBinRetentionDays"`
	NotifyNoteDeletionByPrimary    *bool                  `json:"notifyNoteDeletionByPrimary"`
	NotifyCommentDeletionByPrimary *bool                  `json:"notifyCommentDeletionByPrimary"`
	VoceChatConfig                 map[string]interface{} `json:"voceChatConfig"`
}
