package specpaths

import "fmt"

const (
	ModulesRootDir           = "docs/specs/units"
	CandidateDir             = ModulesRootDir + "/candidate"
	StableDir                = ModulesRootDir + "/stable"
	CandidateAppendixDir     = CandidateDir + "/appendix"
	StableAppendixDir        = StableDir + "/appendix"
)

func MainSpecFileRef(layer, unit string) (string, error) {
	return ObjectMainSpecFileRef("unit", layer, unit)
}

func ObjectMainSpecFileRef(objectType, layer, object string) (string, error) {
	switch layer {
	case "candidate":
		if objectType == "unit" {
			return fmt.Sprintf("%s/c_unit_%s.md", CandidateDir, object), nil
		}
	case "stable":
		if objectType == "unit" {
			return fmt.Sprintf("%s/s_unit_%s.md", StableDir, object), nil
		}
	}
	return "", fmt.Errorf("unsupported object/layer combination %q/%q", objectType, layer)
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
	return fmt.Sprintf("%s/c_unit_%s_*.md", CandidateAppendixDir, unit)
}

func ObjectCandidateAppendixGlob(objectType, object string) (string, error) {
	switch objectType {
	case "unit":
		return fmt.Sprintf("%s/c_unit_%s_*.md", CandidateAppendixDir, object), nil
	default:
		return "", fmt.Errorf("unsupported object type %q", objectType)
	}
}
