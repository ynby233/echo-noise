package services

import (
	"errors"
	"time"

	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrAuthorSubscriptionUnauthorized = errors.New("未登录或用户已不存在")
	ErrAuthorSubscriptionNotFound     = errors.New("作者不存在")
	ErrAuthorSubscriptionSelf         = errors.New("不能订阅自己")
)

type AuthorSubscriptionState struct {
	AuthorID   uint `json:"author_id"`
	Subscribed bool `json:"subscribed"`
}

func validateAuthorSubscriptionUsers(db *gorm.DB, subscriberID, authorID uint) error {
	if subscriberID == 0 {
		return ErrAuthorSubscriptionUnauthorized
	}
	var subscriber models.User
	if err := db.Select("id").First(&subscriber, subscriberID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrAuthorSubscriptionUnauthorized
		}
		return err
	}
	if authorID == subscriberID {
		return ErrAuthorSubscriptionSelf
	}
	if authorID == 0 {
		return ErrAuthorSubscriptionNotFound
	}
	var author models.User
	if err := db.Select("id").First(&author, authorID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrAuthorSubscriptionNotFound
		}
		return err
	}
	return nil
}

// ActiveAuthorSubscriptions excludes orphaned rows even if a user was removed
// outside the normal account-deletion transaction.
func ActiveAuthorSubscriptions(db *gorm.DB) *gorm.DB {
	return db.Model(&models.AuthorSubscription{}).
		Where("subscriber_user_id IN (?)", db.Model(&models.User{}).Select("id")).
		Where("author_user_id IN (?)", db.Model(&models.User{}).Select("id"))
}

func GetAuthorSubscriptionState(db *gorm.DB, subscriberID, authorID uint) (AuthorSubscriptionState, error) {
	state := AuthorSubscriptionState{AuthorID: authorID}
	if err := validateAuthorSubscriptionUsers(db, subscriberID, authorID); err != nil {
		return state, err
	}
	var count int64
	err := ActiveAuthorSubscriptions(db).
		Where("subscriber_user_id = ? AND author_user_id = ?", subscriberID, authorID).
		Count(&count).Error
	state.Subscribed = count > 0
	return state, err
}

func SetAuthorSubscriptionState(db *gorm.DB, subscriberID, authorID uint, subscribed bool) (AuthorSubscriptionState, error) {
	state := AuthorSubscriptionState{AuthorID: authorID}
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := validateAuthorSubscriptionUsers(tx, subscriberID, authorID); err != nil {
			return err
		}
		if subscribed {
			return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&models.AuthorSubscription{
				SubscriberUserID: subscriberID, AuthorUserID: authorID, CreatedAt: time.Now().UTC(),
			}).Error
		}
		return tx.Where("subscriber_user_id = ? AND author_user_id = ?", subscriberID, authorID).
			Delete(&models.AuthorSubscription{}).Error
	})
	if err == nil {
		state.Subscribed = subscribed
	}
	return state, err
}
