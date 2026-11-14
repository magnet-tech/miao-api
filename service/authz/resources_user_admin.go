package authz

// Fork feature resources: administrators act on the users they manage.
const (
	ResourceUserLimit             = "user_limit"
	ResourceUserToken             = "user_token"
	ResourceUserEmail             = "user_email"
	ResourceUserEmailVerification = "user_email_verification"

	ActionReset  = "reset"
	ActionManage = "manage"
	ActionSend   = "send"
)

var (
	UserLimitRead  = Permission{Resource: ResourceUserLimit, Action: ActionRead}
	UserLimitWrite = Permission{Resource: ResourceUserLimit, Action: ActionWrite}
	UserLimitReset = Permission{Resource: ResourceUserLimit, Action: ActionReset}

	UserTokenRead       = Permission{Resource: ResourceUserToken, Action: ActionRead}
	UserTokenManage     = Permission{Resource: ResourceUserToken, Action: ActionManage}
	UserTokenSecretView = Permission{Resource: ResourceUserToken, Action: ActionSecretView}

	UserEmailSend = Permission{Resource: ResourceUserEmail, Action: ActionSend}

	UserEmailVerificationWrite = Permission{Resource: ResourceUserEmailVerification, Action: ActionWrite}
)

func init() {
	RegisterResource(ResourceDefinition{
		Resource: ResourceUserLimit,
		LabelKey: "User Usage Limits",
		Actions: []ActionDefinition{
			{
				Action:         ActionRead,
				LabelKey:       "View usage limits",
				DescriptionKey: "View a user's usage limit rules, windows and concurrency.",
				DefaultRoles:   []string{BuiltInRoleAdmin},
			},
			{
				Action:         ActionWrite,
				LabelKey:       "Edit usage limits",
				DescriptionKey: "Change usage limit rules and the concurrency cap.",
				DefaultRoles:   []string{BuiltInRoleAdmin},
			},
			{
				Action:         ActionReset,
				LabelKey:       "Reset usage windows",
				DescriptionKey: "End a current usage window early.",
				DefaultRoles:   []string{BuiltInRoleAdmin},
			},
		},
	})
	RegisterResource(ResourceDefinition{
		Resource: ResourceUserToken,
		LabelKey: "Managed User API Keys",
		Actions: []ActionDefinition{
			{
				Action:         ActionRead,
				LabelKey:       "View users' API keys",
				DescriptionKey: "List API keys of users you manage, with keys masked.",
				DefaultRoles:   []string{BuiltInRoleAdmin},
			},
			{
				Action:         ActionManage,
				LabelKey:       "Manage users' API keys",
				DescriptionKey: "Create, edit, enable, disable and delete API keys of users you manage.",
				DefaultRoles:   []string{BuiltInRoleAdmin},
			},
			{
				Action:         ActionSecretView,
				LabelKey:       "View users' full API keys",
				DescriptionKey: "Reveal complete API keys after security verification.",
			},
		},
	})
	RegisterResource(ResourceDefinition{
		Resource: ResourceUserEmail,
		LabelKey: "User Emails",
		Actions: []ActionDefinition{{
			Action:         ActionSend,
			LabelKey:       "Send emails to users",
			DescriptionKey: "Send emails and account notices to users you manage.",
			DefaultRoles:   []string{BuiltInRoleAdmin},
		}},
	})
	RegisterResource(ResourceDefinition{
		Resource: ResourceUserEmailVerification,
		LabelKey: "Email Verification Requirement",
		Actions: []ActionDefinition{{
			Action:         ActionWrite,
			LabelKey:       "Require email verification",
			DescriptionKey: "Turn the verified email requirement on or off for users you manage.",
			DefaultRoles:   []string{BuiltInRoleAdmin},
		}},
	})
}
