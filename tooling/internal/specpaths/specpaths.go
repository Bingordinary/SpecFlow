package specpaths

import "fmt"

const (
	ModulesRootDir           = "docs/specs/units"
	CandidateDir             = ModulesRootDir + "/candidate"
	StableDir                = ModulesRootDir + "/stable"
	CandidateAppendixDir     = CandidateDir + "/appendix"
	StableAppendixDir        = StableDir + "/appendix"

	RuleModulesRootDir       = "docs/specs/rules"
	RuleCandidateDir         = RuleModulesRootDir + "/candidate"
	RuleStableDir            = RuleModulesRootDir + "/stable"
)

func MainSpecFileRef(layer, unit string) (string, error) {
	return ObjectMainSpecFileRef("unit", layer, unit)
}

func ObjectMainSpecFileRef(objectType, layer, object string) (string, error) {
	switch objectType {
	case "unit":
		switch layer {
		case "candidate":
			return fmt.Sprintf("%s/unit_%s.md", CandidateDir, object), nil
		case "stable":
			return fmt.Sprintf("%s/unit_%s.md", StableDir, object), nil
		}
	case "rule":
		switch layer {
		case "candidate":
			return fmt.Sprintf("%s/%s.md", RuleCandidateDir, object), nil
		case "stable":
			return fmt.Sprintf("%s/%s.md", RuleStableDir, object), nil
		}
	}
	return "", fmt.Errorf("unsupported object/layer combination %q/%q", objectType, layer)
}

func RuleCandidateFileRef(ruleID string) string {
	ref, err := ObjectMainSpecFileRef("rule", "candidate", ruleID)
	if err != nil {
		panic(err)
	}
	return ref
}

func RuleStableFileRef(ruleID string) string {
	ref, err := ObjectMainSpecFileRef("rule", "stable", ruleID)
	if err != nil {
		panic(err)
	}
	return ref
}

func AppendixDir(layer string) (string, error) {
	switch layer {
	case "candidate":
		return CandidateAppendixDir, nil
	case "stable":
		return StableAppendixDir, nil
	default:
		return "", fmt.Errorf("unsupported layer %q", layer)
	}
}

func CandidateAppendixGlob(unit string) string {
	return fmt.Sprintf("%s/unit_%s_*.md", CandidateAppendixDir, unit)
}

func ObjectCandidateAppendixGlob(objectType, object string) (string, error) {
	switch objectType {
	case "unit":
		return fmt.Sprintf("%s/unit_%s_*.md", CandidateAppendixDir, object), nil
	default:
		return "", fmt.Errorf("unsupported object type %q", objectType)
	}
}
