import type { FormEvent, ReactElement } from "react";
import { useState } from "react";
import {
	createDotfileResource,
	deleteResource,
	OwnerSessionRequiredError,
	updateResource,
} from "./api";
import type { ApiResource } from "./apiTypes";

type DotfileResourceEditorProps = {
	readonly isSaving: boolean;
	readonly onOwnerSessionRequired?: () => void;
	readonly onSaved: () => void;
	readonly onSavingChange: (next: boolean) => void;
	readonly onMessageChange: (next: string) => void;
	readonly resources: readonly ApiResource[];
};

export function DotfileResourceEditor({
	isSaving,
	onOwnerSessionRequired,
	onSaved,
	onSavingChange,
	onMessageChange,
	resources,
}: DotfileResourceEditorProps): ReactElement {
	const dotfiles = resources.filter((resource) => resource.kind === "dotfile");
	const [selectedDotfileId, setSelectedDotfileId] = useState("");
	const [path, setPath] = useState("~/.zshrc");
	const [content, setContent] = useState("");
	const [mode, setMode] = useState("0644");
	const [applyMode, setApplyMode] = useState<"copy" | "symlink">("copy");

	function selectDotfile(resourceId: string): void {
		setSelectedDotfileId(resourceId);
		const resource = dotfiles.find((item) => item.id === resourceId);
		if (resource === undefined) {
			return;
		}
		setPath(specString(resource, "path", resource.name));
		setContent(specString(resource, "content", ""));
		setMode(specString(resource, "mode", "0644"));
		setApplyMode(
			specString(resource, "applyMode", "copy") === "symlink"
				? "symlink"
				: "copy",
		);
	}

	async function submitDotfile(
		event: FormEvent<HTMLFormElement>,
	): Promise<void> {
		event.preventDefault();
		if (selectedDotfileId !== "") {
			await updateSelected();
			return;
		}
		await saveDotfile(() =>
			createDotfileResource({
				path,
				content,
				mode,
				applyMode,
				targetSegment: "base",
			}),
		);
	}

	async function updateSelected(): Promise<void> {
		if (selectedDotfileId === "") {
			return;
		}
		await saveDotfile(() =>
			updateResource(selectedDotfileId, {
				path,
				content,
				mode,
				applyMode,
				targetSegment: "base",
			}),
		);
	}

	async function deleteSelected(): Promise<void> {
		if (selectedDotfileId === "") {
			return;
		}
		onSavingChange(true);
		onMessageChange("");
		try {
			await deleteResource(selectedDotfileId);
			setSelectedDotfileId("");
			onMessageChange("삭제했습니다");
			onSaved();
		} catch (error) {
			handleError(error);
		} finally {
			onSavingChange(false);
		}
	}

	async function saveDotfile(action: () => Promise<unknown>): Promise<void> {
		onSavingChange(true);
		onMessageChange("");
		try {
			await action();
			onMessageChange("저장했습니다");
			onSaved();
		} catch (error) {
			handleError(error);
		} finally {
			onSavingChange(false);
		}
	}

	function handleError(error: unknown): void {
		if (error instanceof OwnerSessionRequiredError) {
			onOwnerSessionRequired?.();
			return;
		}
		onMessageChange(
			error instanceof Error ? error.message : "저장하지 못했습니다",
		);
	}

	return (
		<form className="editor-form" onSubmit={submitDotfile}>
			{dotfiles.length === 0 ? null : (
				<label>
					<span>Existing dotfile</span>
					<select
						aria-label="Existing dotfile"
						value={selectedDotfileId}
						onChange={(event) => selectDotfile(event.target.value)}
					>
						<option value="">New dotfile</option>
						{dotfiles.map((resource) => (
							<option key={resource.id} value={resource.id}>
								{resource.name}
							</option>
						))}
					</select>
				</label>
			)}
			<label>
				<span>Dotfile path</span>
				<input
					aria-label="Dotfile path"
					value={path}
					onChange={(event) => setPath(event.target.value)}
					onInput={(event) => setPath(event.currentTarget.value)}
				/>
			</label>
			<label>
				<span>Dotfile content</span>
				<textarea
					aria-label="Dotfile content"
					value={content}
					onChange={(event) => setContent(event.target.value)}
					onInput={(event) => setContent(event.currentTarget.value)}
				/>
			</label>
			<label>
				<span>Mode</span>
				<input
					aria-label="Dotfile mode"
					value={mode}
					onChange={(event) => setMode(event.target.value)}
					onInput={(event) => setMode(event.currentTarget.value)}
				/>
			</label>
			<label>
				<span>Apply</span>
				<select
					aria-label="Dotfile apply mode"
					value={applyMode}
					onChange={(event) =>
						setApplyMode(event.target.value === "symlink" ? "symlink" : "copy")
					}
				>
					<option value="copy">copy</option>
					<option value="symlink">symlink</option>
				</select>
			</label>
			<button className="primary-button" type="submit" disabled={isSaving}>
				{selectedDotfileId === "" ? "Save dotfile" : "Update dotfile"}
			</button>
			{selectedDotfileId === "" ? null : (
				<button
					className="secondary-button danger-button"
					type="button"
					disabled={isSaving}
					onClick={deleteSelected}
				>
					Delete dotfile
				</button>
			)}
		</form>
	);
}

function specString(
	resource: ApiResource,
	key: string,
	fallback: string,
): string {
	const value = resource.spec[key];
	return typeof value === "string" ? value : fallback;
}
