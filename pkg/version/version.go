package version

import (
	"fmt"
	"regexp"
	"runtime"
	"strconv"
)

const (
	maxCommitLength = 7
	versionElements = 5

	errMsg = "failed to cast %q version in semantic version %q to int: %w"
)

var (
	regExp         = regexp.MustCompile(`(\d*)\.(\d*)\.(\d*)(.*)`)
	serviceVersion = version{}
)

// The Init sets the binaries version once and for all. This comes from build time inputs.
func Init(semVersion, buildDate, gitCommit, gitTag, gitTreeState string) error {
	matches := regExp.FindStringSubmatch(semVersion)
	if len(matches) != versionElements {
		return fmt.Errorf("illegal version provided: %q", semVersion)
	}

	serviceVersion = version{
		version:      semVersion,
		buildDate:    buildDate,
		gitCommit:    gitCommit,
		gitTag:       gitTag,
		gitTreeState: gitTreeState,
		goVersion:    runtime.Version(),
		compiler:     runtime.Compiler,
		platform:     fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}

	var err error

	serviceVersion.major, err = strconv.Atoi(matches[1])
	if err != nil {
		return fmt.Errorf(errMsg, "major", semVersion, err)
	}
	serviceVersion.minor, err = strconv.Atoi(matches[2])
	if err != nil {
		return fmt.Errorf(errMsg, "minor", semVersion, err)
	}
	serviceVersion.patch, err = strconv.Atoi(matches[3])
	if err != nil {
		return fmt.Errorf(errMsg, "patch", semVersion, err)
	}

	serviceVersion.suffix = matches[4]

	return nil
}

// Read returns the version string. This is either the gitTag for released version
// or a combination of version number, commit hash and gitTreeState.
func Read() string {
	if serviceVersion.gitCommit != "" &&
		serviceVersion.gitTag != "" &&
		serviceVersion.gitTreeState == "clean" {
		// if we have a clean tree state and the current commit is tagged,
		// this is an official release.
		return serviceVersion.gitTag
	}
	// otherwise formulate a version string based on as much metadata
	// information we have available.
	return "v" + serviceVersion.version + serviceVersion.commitInfo()
}

func SemanticVersion() (major, minor, patch int) {
	return serviceVersion.major, serviceVersion.minor, serviceVersion.patch
}

// ReadAll returns the complete version information.
func ReadAll() string {
	return fmt.Sprintf("%+v", serviceVersion)
}

// version contains version information.
type version struct {
	version      string
	major        int
	minor        int
	patch        int
	suffix       string
	buildDate    string
	gitCommit    string
	gitTag       string
	gitTreeState string
	goVersion    string
	compiler     string
	platform     string
}

func (v *version) commitInfo() string {
	if len(v.gitCommit) >= maxCommitLength {
		res := "+" + v.gitCommit[0:maxCommitLength]
		if v.gitTreeState != "clean" {
			res += ".dirty"
		}

		return res
	}

	return "+unknown"
}
