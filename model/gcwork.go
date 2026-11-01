package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// GcworkManagedTokenName is the name a user sees on the token GC Work manages.
const GcworkManagedTokenName = "GC Work"

// GcworkManagedToken marks the relay token GC Work manages for a user. Each
// user has at most one; it never expires, and the user can neither change it
// nor read its key. The marker lives in its own table so the upstream token
// table and its queries stay unchanged.
type GcworkManagedToken struct {
	UserId      int   `json:"user_id" gorm:"primaryKey;autoIncrement:false"`
	TokenId     int   `json:"token_id" gorm:"uniqueIndex"`
	CreatedTime int64 `json:"created_time" gorm:"bigint"`
}

// EnsureGcworkManagedToken returns the user's managed token, creating it on
// first use. Creation ignores the per-user token limit: a user's own tokens must
// not lock them out of GC Work. Across nodes the primary key decides the winner.
func EnsureGcworkManagedToken(userId int) (*Token, error) {
	token, err := currentGcworkManagedToken(userId)
	if token != nil || err != nil {
		return token, err
	}
	key, err := common.GenerateKey()
	if err != nil {
		return nil, err
	}
	now := common.GetTimestamp()
	token = &Token{
		UserId:              userId,
		Name:                GcworkManagedTokenName,
		Key:                 key,
		CreatedTime:         now,
		AccessedTime:        now,
		ExpiredTime:         -1,
		UnlimitedQuota:      true,
		AllowUserQuotaQuery: true,
	}
	createErr := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(token).Error; err != nil {
			return err
		}
		return tx.Create(&GcworkManagedToken{UserId: userId, TokenId: token.Id, CreatedTime: now}).Error
	})
	if createErr == nil {
		return token, nil
	}
	// Another node may have created the token meanwhile.
	existing, err := currentGcworkManagedToken(userId)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, createErr
	}
	return existing, nil
}

// currentGcworkManagedToken returns the user's enabled managed token, or nil
// when the user has none. It drops a marker whose token no longer exists, for
// example after an administrator removed the user's tokens directly.
func currentGcworkManagedToken(userId int) (*Token, error) {
	var marker GcworkManagedToken
	err := DB.Where("user_id = ?", userId).First(&marker).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	token, err := GetTokenById(marker.TokenId)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, DB.Delete(&marker).Error
	}
	if err != nil {
		return nil, err
	}
	if token.Status != common.TokenStatusEnabled {
		token.Status = common.TokenStatusEnabled
		if err := token.SelectUpdate(); err != nil {
			return nil, err
		}
	}
	return token, nil
}

// DeleteGcworkManagedToken deletes the user's managed token, so its key stops
// working. A user without one succeeds. The next ensure issues a different key.
func DeleteGcworkManagedToken(userId int) error {
	var marker GcworkManagedToken
	err := DB.Where("user_id = ?", userId).First(&marker).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	token, err := GetTokenById(marker.TokenId)
	if err == nil {
		err = token.Delete()
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return DB.Delete(&marker).Error
}

// GcworkManagedTokenId returns the id of the user's managed token, or 0.
func GcworkManagedTokenId(userId int) (int, error) {
	var marker GcworkManagedToken
	err := DB.Where("user_id = ?", userId).First(&marker).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	return marker.TokenId, err
}

// AnyGcworkManagedToken reports whether any of the token ids is a managed token.
func AnyGcworkManagedToken(ids []int) (bool, error) {
	if len(ids) == 0 {
		return false, nil
	}
	var count int64
	err := DB.Model(&GcworkManagedToken{}).Where("token_id IN ?", ids).Count(&count).Error
	return count > 0, err
}
