// SPDX-License-Identifier: AGPL-3.0-or-later

package model

import (
	"time"
)

// VisitDedupWindow is the minimum time between two recorded visits of the
// same URL by the same user. Browser extensions can submit a page several
// times while it loads, which must not be counted as separate visits.
const VisitDedupWindow = 30 * time.Second

// DocumentVisit records a single visit of a document URL by a user.
type DocumentVisit struct {
	ID        uint      `gorm:"primaryKey"                                      json:"id"`
	URL       string    `gorm:"index:idx_visit_url_user;not null"               json:"url"`
	UserID    uint      `gorm:"index:idx_visit_url_user;index:idx_visit_user_time" json:"user_id"`
	VisitedAt time.Time `gorm:"index:idx_visit_user_time;not null"              json:"visited_at"`
}

// RecordVisit stores a visit of url by userID at the given time. A zero time
// records the current time. Visits that fall within VisitDedupWindow of an
// existing visit of the same URL are ignored. It reports whether a new visit
// was stored.
func RecordVisit(url string, userID uint, at time.Time) (bool, error) {
	if url == "" {
		return false, nil
	}
	if at.IsZero() {
		at = time.Now()
	}
	at = at.UTC()
	var count int64
	if err := DB.Model(&DocumentVisit{}).
		Where("url = ? AND user_id = ? AND visited_at > ? AND visited_at < ?",
			url, userID, at.Add(-VisitDedupWindow), at.Add(VisitDedupWindow)).
		Count(&count).Error; err != nil {
		return false, err
	}
	if count > 0 {
		return false, nil
	}
	if err := DB.Create(&DocumentVisit{URL: url, UserID: userID, VisitedAt: at}).Error; err != nil {
		return false, err
	}
	return true, nil
}

// GetVisits returns visits of a URL by a user, newest first. A limit below
// one returns every visit.
func GetVisits(url string, userID uint, limit, offset int) ([]*DocumentVisit, error) {
	var visits []*DocumentVisit
	q := DB.Where("url = ? AND user_id = ?", url, userID).Order("visited_at DESC").Offset(offset)
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&visits).Error; err != nil {
		return nil, err
	}
	return visits, nil
}

// CountVisits returns the number of recorded visits of a URL by a user.
func CountVisits(url string, userID uint) (int64, error) {
	var count int64
	err := DB.Model(&DocumentVisit{}).Where("url = ? AND user_id = ?", url, userID).Count(&count).Error
	return count, err
}

// DeleteVisits removes every recorded visit of a URL by a user.
func DeleteVisits(url string, userID uint) error {
	return DB.Where("url = ? AND user_id = ?", url, userID).Delete(&DocumentVisit{}).Error
}

// DeleteVisit removes a single recorded visit owned by a user.
func DeleteVisit(id, userID uint) (bool, error) {
	res := DB.Where("id = ? AND user_id = ?", id, userID).Delete(&DocumentVisit{})
	return res.RowsAffected > 0, res.Error
}

// MoveVisits assigns the visits of a document to a new owner.
func MoveVisits(url string, fromUserID, toUserID uint) error {
	if fromUserID == toUserID {
		return nil
	}
	return DB.Model(&DocumentVisit{}).
		Where("url = ? AND user_id = ?", url, fromUserID).
		Update("user_id", toUserID).Error
}
