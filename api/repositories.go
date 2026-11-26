package api

// RepositoryView is one of a Project's Repositories as every Project read
// returns it, in landing order.
type RepositoryView struct {
	Name string `json:"name"`
	// Home marks the Home Repository, named by the Project Key.
	Home bool `json:"home"`
	// CloneURL is the remote the Repository is cloned from and pushed to:
	// the Push remote URL the Factory reported for the Home Repository,
	// empty until it has, or when it reported none.
	CloneURL string `json:"clone_url"`
	// LocalOnly marks a Repository with no Clone URL, never cloned or
	// pushed: the Home Repository once the Factory reports its clone has
	// no Push remote, and not before.
	LocalOnly bool `json:"local_only"`
	// Base is the branch its work is cut from and lands on: the Project's
	// Base for the Home Repository.
	Base string `json:"base"`
	// LocalPath is where the Factory finds its clone, empty for beside
	// the Home Repository.
	LocalPath string `json:"local_path"`
	// CodeHost is the Code Host its Clone URL names, empty for none
	// Code Central supports.
	CodeHost CodeHost `json:"code_host"`
	// ChangedBy are the unfinished Tickets whose Changed Repositories
	// include it, which keep it from being removed. Only get_project and
	// list_projects fill them in.
	ChangedBy []string `json:"changed_by"`
	// Clone is its clone as the Factory last reported it, absent while no
	// Factory is reporting.
	Clone *CloneView `json:"clone,omitempty"`
	// Landing is its effective Landing: its Own Landing, else the
	// Project's Default Landing; never default.
	Landing RepositoryLanding `json:"landing"`
	// OwnLanding is whether Landing is its Own Landing.
	OwnLanding bool `json:"own_landing"`
	// PushRemote is the git remote it is pushed to and fetched from: its
	// own, else the Project's.
	PushRemote string `json:"push_remote"`
	// OwnPushRemote is whether PushRemote is its own.
	OwnPushRemote bool `json:"own_push_remote"`
	// CheckCommand is the Check Command an agent runs in it: its own,
	// else the Project's, else empty for none.
	CheckCommand string `json:"check_command"`
	// OwnCheckCommand is whether CheckCommand is its own.
	OwnCheckCommand bool `json:"own_check_command"`
	// LocalOnlyUnderPullRequest marks a local-only Repository whose
	// Landing is Pull Request, as only a Home that lost its remote can
	// be: nothing lands there until it has another Landing or a remote.
	LocalOnlyUnderPullRequest bool `json:"local_only_under_pull_request"`
	// CI is its CI choice, the Home's included: its CI entry, stored or
	// filled in from its Clone URL.
	CI CIRepositoryView `json:"ci"`
}

// AddRepositoryInput is the human's add_repository(name, clone_url?,
// base?, local_path?, ci?, landing?, push_remote?, check_command?,
// full?): it needs
// clone_url, local_path or both.
type AddRepositoryInput struct {
	// Name is the Repository Name, fixed once added.
	Name string `json:"name"`
	// CloneURL left out makes the Repository local-only: the Factory
	// never clones or pushes it, and adopts the clone at LocalPath.
	CloneURL string `json:"clone_url,omitempty"`
	// Base is main when left out.
	Base string `json:"base,omitempty"`
	// LocalPath is an absolute path to an existing clone; left out, the
	// Factory clones it beside the Home Repository.
	LocalPath string `json:"local_path,omitempty"`
	// CI is the Repository's CI entry; left out, it is filled in from the
	// Clone URL, Not watched for none or a host no CI Provider is on.
	CI *RepositoryCI `json:"ci,omitempty"`
	// Landing is its Own Landing; left out or default, it follows the
	// Project's Default Landing.
	Landing RepositoryLanding `json:"landing,omitempty"`
	// PushRemote names the git remote it is pushed to; left out, the
	// Project's.
	PushRemote string `json:"push_remote,omitempty"`
	// CheckCommand is its own Check Command; left out, the Project's.
	CheckCommand string `json:"check_command,omitempty"`
	// Full asks for the Project in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// EditRepositoryInput is the human's edit_repository(name, clone_url?,
// base?, local_path?, landing?, push_remote?, check_command?, full?): each
// field left out keeps its value. The name only finds the Repository: a
// Repository Name never changes. The Home Repository takes the landing,
// push_remote and check_command alone.
type EditRepositoryInput struct {
	Name string `json:"name"`
	// CloneURL, set empty, clears it, making the Repository local-only.
	CloneURL *string `json:"clone_url,omitempty"`
	Base     *string `json:"base,omitempty"`
	// LocalPath, set empty, puts the clone back beside the Home
	// Repository. A Repository keeps a Clone URL or a local path.
	LocalPath *string `json:"local_path,omitempty"`
	// Landing, set default, has it follow the Project's Default Landing;
	// any other value is its Own Landing.
	Landing *RepositoryLanding `json:"landing,omitempty"`
	// PushRemote, set empty, has it use the Project's.
	PushRemote *string `json:"push_remote,omitempty"`
	// CheckCommand, set empty, has it use the Project's.
	CheckCommand *string `json:"check_command,omitempty"`
	// Full asks for the Project in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// RemoveRepositoryInput is the human's remove_repository(name, full?).
type RemoveRepositoryInput struct {
	Name string `json:"name"`
	// Full asks for the Project in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// SetLandingOrderInput is the human's set_landing_order(names, full?):
// every Repository Name once, in the new landing order.
type SetLandingOrderInput struct {
	Names []string `json:"names"`
	// Full asks for the Project in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}
