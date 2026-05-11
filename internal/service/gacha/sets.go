package gacha

import (
	"gachabot/internal/models"
)

func (s *GachaService) GetUserSetsProgress(internalUserID int64) ([]models.UserSetProgress, error) {
	return s.repo.GetUserSetsProgress(internalUserID)
}

func (s *GachaService) GetSetCards(internalUserID int64, setID int) ([]models.Card, error) {
	return s.repo.GetSetCards(internalUserID, setID)
}

func (s *GachaService) EquipSetAura(internalUserID int64, setID *int) error {
	return s.repo.EquipSetAura(internalUserID, setID)
}
