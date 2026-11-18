package authz

import (
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

const (
	ResourceModelReplacement = "model_replacement"

	ActionConfigure = "configure"
)

var (
	ModelReplacementRead      = Permission{Resource: ResourceModelReplacement, Action: ActionRead}
	ModelReplacementOperate   = Permission{Resource: ResourceModelReplacement, Action: ActionOperate}
	ModelReplacementConfigure = Permission{Resource: ResourceModelReplacement, Action: ActionConfigure}
)

func init() {
	RegisterResource(ResourceDefinition{
		Resource: ResourceModelReplacement,
		LabelKey: "Model Replacement Detection",
		Actions: []ActionDefinition{
			{
				Action:         ActionRead,
				LabelKey:       "View detection results",
				DescriptionKey: "View the overview, channel results and history.",
			},
			{
				Action:         ActionOperate,
				LabelKey:       "Run detection",
				DescriptionKey: "Run detection now, retest channels and run trials.",
			},
			{
				Action:         ActionConfigure,
				LabelKey:       "Configure detection",
				DescriptionKey: "Change detection settings and the actions applied to channels.",
			},
		},
	})
}

// legacyModelReplacementAction is how the first fork release stored the grant:
// a single extra action on the channel resource.
const legacyModelReplacementAction = "model_replacement"

// migrateLegacyModelReplacementGrants turns each stored channel/model_replacement
// override into the same effect on every action of the model_replacement
// resource, then removes the old row. It is safe to run on every startup.
func migrateLegacyModelReplacementGrants(db *gorm.DB) error {
	var legacy []model.CasbinRule
	err := db.Where("ptype = ? AND v1 = ? AND v2 = ?", "p", ResourceChannel, legacyModelReplacementAction).
		Find(&legacy).Error
	if err != nil || len(legacy) == 0 {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		adapter := newGormAdapter(tx)
		for _, rule := range legacy {
			for _, action := range []string{ActionRead, ActionOperate, ActionConfigure} {
				if err := adapter.AddPolicy("", "p", []string{rule.V0, ResourceModelReplacement, action, rule.V3}); err != nil {
					return err
				}
			}
			if err := tx.Delete(&model.CasbinRule{}, rule.Id).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
