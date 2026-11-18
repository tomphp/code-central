// Code in this file is temporary: the aliases let every caller keep
// naming store.X while the records live in domain (Spec code-central-925).
// They go once every caller names domain.X.

package store

import "github.com/tomoram/code-central/domain"

type (
	AutoAcceptCause   = domain.AutoAcceptCause
	AutoAcceptKind    = domain.AutoAcceptKind
	AzureBoards       = domain.AzureBoards
	AzureBoardsMode   = domain.AzureBoardsMode
	CI                = domain.CI
	CIConclusion      = domain.CIConclusion
	CIJob             = domain.CIJob
	CIProvider        = domain.CIProvider
	CIRepository      = domain.CIRepository
	CIResponse        = domain.CIResponse
	CIRule            = domain.CIRule
	CIRun             = domain.CIRun
	CIRunAttempt      = domain.CIRunAttempt
	CIRunStatus       = domain.CIRunStatus
	CIRuns            = domain.CIRuns
	ChangedRepository = domain.ChangedRepository
	Claim             = domain.Claim
	Comment           = domain.Comment
	Edit              = domain.Edit
	ExternalState     = domain.ExternalState
	FailureReport     = domain.FailureReport
	FocusClear        = domain.FocusClear
	FocusPopup        = domain.FocusPopup
	GitHubIssues      = domain.GitHubIssues
	KindModels        = domain.KindModels
	Landed            = domain.Landed
	Landing           = domain.Landing
	LandingMethod     = domain.LandingMethod
	LimitsPopup       = domain.LimitsPopup
	Link              = domain.Link
	Mode              = domain.Mode
	ModelRow          = domain.ModelRow
	Models            = domain.Models
	Move              = domain.Move
	Options           = domain.Options
	OwnLanding        = domain.OwnLanding
	Project           = domain.Project
	PullRequest       = domain.PullRequest
	PullRequestState  = domain.PullRequestState
	PushedCommit      = domain.PushedCommit
	Question          = domain.Question
	QuestionStatus    = domain.QuestionStatus
	Recommendation    = domain.Recommendation
	ReportedModels    = domain.ReportedModels
	Repository        = domain.Repository
	Round             = domain.Round
	Spec              = domain.Spec
	SpecMerge         = domain.SpecMerge
	StageGuides       = domain.StageGuides
	State             = domain.State
	Ticket            = domain.Ticket
)

const (
	Accepted                 = domain.Accepted
	Answered                 = domain.Answered
	AnyWorkflow              = domain.AnyWorkflow
	AutoAcceptGrilling       = domain.AutoAcceptGrilling
	AutoAcceptImplementation = domain.AutoAcceptImplementation
	AutoAcceptMerge          = domain.AutoAcceptMerge
	AutoAcceptSpec           = domain.AutoAcceptSpec
	AutoAcceptTickets        = domain.AutoAcceptTickets
	AzureBoardsSavedQuery    = domain.AzureBoardsSavedQuery
	AzureBoardsWIQL          = domain.AzureBoardsWIQL
	CIAzurePipelines         = domain.CIAzurePipelines
	CICancelled              = domain.CICancelled
	CICompleted              = domain.CICompleted
	CIFailure                = domain.CIFailure
	CIGitHubActions          = domain.CIGitHubActions
	CIInProgress             = domain.CIInProgress
	CINeutral                = domain.CINeutral
	CIPartiallySucceeded     = domain.CIPartiallySucceeded
	CIQueued                 = domain.CIQueued
	CISkipped                = domain.CISkipped
	CISuccess                = domain.CISuccess
	CITimedOut               = domain.CITimedOut
	Cancelled                = domain.Cancelled
	CauseProject             = domain.CauseProject
	CauseSpecLabel           = domain.CauseSpecLabel
	CauseTicketLabel         = domain.CauseTicketLabel
	DefaultBase              = domain.DefaultBase
	DefaultLandingRemote     = domain.DefaultLandingRemote
	DefaultPATEnv            = domain.DefaultPATEnv
	DefaultRepositoryBase    = domain.DefaultRepositoryBase
	Done                     = domain.Done
	Draft                    = domain.Draft
	ExternalClosed           = domain.ExternalClosed
	ExternalDeleted          = domain.ExternalDeleted
	ExternalOpen             = domain.ExternalOpen
	ExternalOutOfScope       = domain.ExternalOutOfScope
	Failed                   = domain.Failed
	FocusActivityDashboard   = domain.FocusActivityDashboard
	FocusAllClear            = domain.FocusAllClear
	FocusPopupOff            = domain.FocusPopupOff
	FocusPopupOn             = domain.FocusPopupOn
	InProgress               = domain.InProgress
	InReview                 = domain.InReview
	LandingBranch            = domain.LandingBranch
	LandingMerge             = domain.LandingMerge
	LandingPullRequest       = domain.LandingPullRequest
	LimitsPopupOff           = domain.LimitsPopupOff
	LimitsPopupOn            = domain.LimitsPopupOn
	MaxRepositoryName        = domain.MaxRepositoryName
	ModeCommitted            = domain.ModeCommitted
	ModeLocal                = domain.ModeLocal
	NeedsAnswer              = domain.NeedsAnswer
	PRClosed                 = domain.PRClosed
	PRConflicting            = domain.PRConflicting
	PRMerged                 = domain.PRMerged
	PROpen                   = domain.PROpen
	QuestionAnswered         = domain.QuestionAnswered
	QuestionDismissed        = domain.QuestionDismissed
	QuestionOpen             = domain.QuestionOpen
	Ready                    = domain.Ready
	RecommendImplement       = domain.RecommendImplement
	RecommendSplit           = domain.RecommendSplit
)

var (
	AutoAcceptCauses     = domain.AutoAcceptCauses
	AutoAcceptKinds      = domain.AutoAcceptKinds
	AzureBoardsModes     = domain.AzureBoardsModes
	CIConclusions        = domain.CIConclusions
	CIProviders          = domain.CIProviders
	CIRunStatuses        = domain.CIRunStatuses
	ExternalStates       = domain.ExternalStates
	FocusClears          = domain.FocusClears
	FocusPopups          = domain.FocusPopups
	LandingMethods       = domain.LandingMethods
	LimitsPopups         = domain.LimitsPopups
	ModelKinds           = domain.ModelKinds
	Modes                = domain.Modes
	NeedsMeReasons       = domain.NeedsMeReasons
	PullRequestStates    = domain.PullRequestStates
	QuestionStatuses     = domain.QuestionStatuses
	Recommendations      = domain.Recommendations
	States               = domain.States
	AzureBoardsModeNames = domain.AzureBoardsModeNames
	CIProviderNames      = domain.CIProviderNames
	DefaultCI            = domain.DefaultCI
	DefaultLanding       = domain.DefaultLanding
	DefaultOptions       = domain.DefaultOptions
	FocusClearNames      = domain.FocusClearNames
	FocusPopupNames      = domain.FocusPopupNames
	LandingMethodNames   = domain.LandingMethodNames
	LimitsPopupNames     = domain.LimitsPopupNames
	ValidRepositoryName  = domain.ValidRepositoryName
)
