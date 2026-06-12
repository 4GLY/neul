package agent

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/4gly/neul/internal/domain/dotfiles"
)

type dotfileSpec struct {
	path          string
	content       string
	mode          os.FileMode
	applyMode     string
	targetSegment string
}

func ApplyDotfile(_ context.Context, homeDir string, resource DesiredResource) ResourceEvent {
	if homeDir == "" {
		return blockedDotfileEvent(resource, dotfiles.MessageHomeUnavailable)
	}
	if resource.Spec == nil {
		return blockedDotfileEvent(resource, dotfiles.MessageInvalidSpec)
	}
	rawPath := stringSpec(resource, "path")
	if rawPath == "" {
		return blockedDotfileEvent(resource, dotfiles.MessageInvalidSpec)
	}
	normalizedPath, err := dotfiles.NormalizeAllowedPathSyntax(homeDir, rawPath)
	if err != nil {
		return blockedDotfileEvent(resource, dotfiles.MessageForError(err))
	}
	spec, err := parseDotfileSpec(resource)
	if err != nil {
		return blockedDotfileEvent(resource, dotfiles.MessageInvalidSpec)
	}
	targetPath, err := dotfiles.AbsoluteTarget(homeDir, normalizedPath)
	if err != nil {
		return blockedDotfileEvent(resource, dotfiles.MessageForError(err))
	}
	if err := ensureSafeParent(homeDir, targetPath); err != nil {
		return blockedDotfileEvent(resource, dotfiles.MessageForError(err))
	}
	switch spec.applyMode {
	case "copy":
		err = applyDotfileCopy(homeDir, targetPath, resource.ID, spec)
	case "symlink":
		err = applyDotfileSymlink(homeDir, targetPath, resource.ID, spec)
	default:
		err = dotfiles.NewPolicyError(dotfiles.MessageInvalidSpec, fmt.Errorf("unsupported apply mode %q", spec.applyMode))
	}
	if err != nil {
		return blockedDotfileEvent(resource, dotfiles.MessageForError(err))
	}
	return ResourceEvent{ResourceID: resource.ID, Status: "in_sync", Message: dotfiles.MessageApplied, DesiredVersion: resource.DesiredVersion, AppliedVersion: resource.DesiredVersion}
}

func parseDotfileSpec(resource DesiredResource) (dotfileSpec, error) {
	if resource.Spec == nil || resource.ID == "" {
		return dotfileSpec{}, errors.New("dotfile spec is required")
	}
	path := stringSpec(resource, "path")
	content, ok := resource.Spec["content"].(string)
	if !ok {
		return dotfileSpec{}, errors.New("dotfile content is required")
	}
	modeText := stringSpec(resource, "mode")
	applyMode := stringSpec(resource, "applyMode")
	targetSegment := stringSpec(resource, "targetSegment")
	if !safeManagedSegment(resource.ID) || !safeManagedSegment(targetSegment) {
		return dotfileSpec{}, errors.New("managed path segment is unsafe")
	}
	if err := dotfiles.ValidateSpec(modeText, applyMode, targetSegment); err != nil {
		return dotfileSpec{}, err
	}
	parsedMode, err := parseFileMode(modeText)
	if err != nil {
		return dotfileSpec{}, err
	}
	return dotfileSpec{path: path, content: content, mode: parsedMode, applyMode: applyMode, targetSegment: targetSegment}, nil
}

func applyDotfileCopy(homeDir string, targetPath string, resourceID string, spec dotfileSpec) error {
	managedDir, err := managedDotfileDir(homeDir, resourceID, spec)
	if err != nil {
		return err
	}
	info, err := os.Lstat(targetPath)
	if err == nil {
		if err := handleExistingCopyTarget(homeDir, targetPath, spec, info, managedDir); err != nil {
			return err
		}
		_, managedPath, err := writeManagedDotfile(homeDir, resourceID, spec)
		if err != nil {
			return err
		}
		if err := pruneManagedSiblings(managedDir, managedPath); err != nil {
			return err
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return dotfiles.NewPolicyError(dotfiles.MessageWriteFailed, fmt.Errorf("lstat target: %w", err))
	}
	_, managedPath, err := writeManagedDotfile(homeDir, resourceID, spec)
	if err != nil {
		return err
	}
	if err := writeFileAtomically(homeDir, targetPath, []byte(spec.content), spec.mode); err != nil {
		return err
	}
	body, err := os.ReadFile(targetPath)
	if err != nil {
		return dotfiles.NewPolicyError(dotfiles.MessageWriteFailed, fmt.Errorf("read target: %w", err))
	}
	if string(body) != spec.content {
		return dotfiles.NewPolicyError(dotfiles.MessageWriteFailed, errors.New("written content did not verify"))
	}
	if err := pruneManagedSiblings(managedDir, managedPath); err != nil {
		return err
	}
	return nil
}

func handleExistingCopyTarget(homeDir string, targetPath string, spec dotfileSpec, info os.FileInfo, managedDir string) error {
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		return dotfiles.NewPolicyError(dotfiles.MessageConflictExistingSymlink, errors.New("target is a symlink"))
	case info.IsDir():
		return dotfiles.NewPolicyError(dotfiles.MessageConflictExistingDirectory, errors.New("target is a directory"))
	case !info.Mode().IsRegular():
		return dotfiles.NewPolicyError(dotfiles.MessageConflictExistingNode, errors.New("target is not a regular file"))
	case hasMultipleLinks(info):
		return dotfiles.NewPolicyError(dotfiles.MessageConflictExistingFile, errors.New("target has multiple hard links"))
	}
	body, err := os.ReadFile(targetPath)
	if err != nil {
		return dotfiles.NewPolicyError(dotfiles.MessageWriteFailed, fmt.Errorf("read existing target: %w", err))
	}
	if string(body) != spec.content {
		if !managedContentExists(managedDir, body) {
			return dotfiles.NewPolicyError(dotfiles.MessageConflictExistingFile, errors.New("target has different content"))
		}
		if err := writeFileAtomically(homeDir, targetPath, []byte(spec.content), spec.mode); err != nil {
			return err
		}
		return nil
	}
	if err := os.Chmod(targetPath, spec.mode); err != nil {
		return dotfiles.NewPolicyError(dotfiles.MessageWriteFailed, fmt.Errorf("chmod existing target: %w", err))
	}
	return nil
}

func applyDotfileSymlink(homeDir string, targetPath string, resourceID string, spec dotfileSpec) error {
	managedDir, err := managedDotfileDir(homeDir, resourceID, spec)
	if err != nil {
		return err
	}
	info, targetErr := os.Lstat(targetPath)
	if targetErr == nil && info.Mode()&os.ModeSymlink == 0 {
		return conflictForExistingInfo(info)
	}
	if targetErr != nil && !os.IsNotExist(targetErr) {
		return dotfiles.NewPolicyError(dotfiles.MessageWriteFailed, fmt.Errorf("lstat target: %w", targetErr))
	}
	targetExists := targetErr == nil
	currentTarget := ""
	if targetExists {
		var err error
		currentTarget, err = os.Readlink(targetPath)
		if err != nil {
			return dotfiles.NewPolicyError(dotfiles.MessageWriteFailed, fmt.Errorf("readlink target: %w", err))
		}
		if !managedPathOwnedByResource(managedDir, currentTarget) {
			return dotfiles.NewPolicyError(dotfiles.MessageConflictExistingSymlink, errors.New("target symlink is not managed by this resource"))
		}
	}
	managedDir, managedPath, err := writeManagedDotfile(homeDir, resourceID, spec)
	if err != nil {
		return err
	}
	if targetExists {
		if currentTarget == managedPath {
			return pruneManagedSiblings(managedDir, managedPath)
		}
		if err := replaceSymlink(homeDir, targetPath, managedPath); err != nil {
			return err
		}
		return pruneManagedSiblings(managedDir, managedPath)
	}
	if !os.IsNotExist(targetErr) {
		return dotfiles.NewPolicyError(dotfiles.MessageWriteFailed, fmt.Errorf("lstat target: %w", targetErr))
	}
	if err := replaceSymlink(homeDir, targetPath, managedPath); err != nil {
		return err
	}
	return pruneManagedSiblings(managedDir, managedPath)
}
