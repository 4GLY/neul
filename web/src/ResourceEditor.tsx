import type { FormEvent, ReactElement } from "react";
import { useState } from "react";
import {
	createDotfileResource,
	createPackageResource,
	deleteResource,
	OwnerSessionRequiredError,
	updatePackageResource,
} from "./api";
import { PackageResourceList } from "./PackageResourceList";
import type { ResourceRow } from "./types";

type ResourceEditorProps = {
	readonly onOwnerSessionRequired?: () => void;
	readonly onSaved: () => void;
	readonly resources?: readonly ResourceRow[];
};

export function ResourceEditor({
	onOwnerSessionRequired,
	onSaved,
	resources = [],
}: ResourceEditorProps): ReactElement {
	const [mode, setMode] = useState<"package" | "dotfile">("package");
	const [editingResourceId, setEditingResourceId] = useState("");
	const [packageName, setPackageName] = useState("");
	const [sourceKind, setSourceKind] = useState<"brew" | "apt" | "mise">("brew");
	const [packageVersion, setPackageVersion] = useState("latest");
	const [dotfilePath, setDotfilePath] = useState("~/.zshrc");
	const [dotfileContent, setDotfileContent] = useState("");
	const [message, setMessage] = useState("");
	const [isSaving, setIsSaving] = useState(false);

	async function handlePackageSubmit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		setIsSaving(true);
		setMessage("");
		try {
			const input = {
				name: packageName,
				sourceKind,
				desiredVersion: packageVersion,
			};
			if (editingResourceId === "") {
				await createPackageResource({ ...input, targetSegment: "base" });
			} else {
				await updatePackageResource(editingResourceId, input);
			}
			setMessage("저장했습니다");
			resetPackageEdit();
			onSaved();
		} catch (error) {
			if (error instanceof OwnerSessionRequiredError) {
				onOwnerSessionRequired?.();
				return;
			}
			setMessage(
				error instanceof Error ? error.message : "저장하지 못했습니다",
			);
		} finally {
			setIsSaving(false);
		}
	}

	async function handleDotfileSubmit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		setIsSaving(true);
		setMessage("");
		try {
			await createDotfileResource({
				path: dotfilePath,
				content: dotfileContent,
				mode: "0644",
				applyMode: "copy",
				targetSegment: "base",
			});
			setMessage("저장했습니다");
			setEditingResourceId("");
			onSaved();
		} catch (error) {
			if (error instanceof OwnerSessionRequiredError) {
				onOwnerSessionRequired?.();
				return;
			}
			setMessage(
				error instanceof Error ? error.message : "저장하지 못했습니다",
			);
		} finally {
			setIsSaving(false);
		}
	}

	async function handleDeletePackage(resourceId: string) {
		if (!window.confirm("Delete this package desired state?")) {
			return;
		}
		setIsSaving(true);
		setMessage("");
		try {
			await deleteResource(resourceId);
			if (editingResourceId === resourceId) {
				setEditingResourceId("");
			}
			setMessage("삭제했습니다");
			onSaved();
		} catch (error) {
			if (error instanceof OwnerSessionRequiredError) {
				onOwnerSessionRequired?.();
				return;
			}
			setMessage(
				error instanceof Error ? error.message : "삭제하지 못했습니다",
			);
		} finally {
			setIsSaving(false);
		}
	}

	function startPackageEdit(resource: ResourceRow) {
		setMode("package");
		setEditingResourceId(resource.id);
		setPackageName(resource.name);
		setSourceKind(resource.sourceKind ?? "brew");
		setPackageVersion(resource.desired);
		setMessage("");
	}

	function resetPackageEdit() {
		setEditingResourceId("");
		setPackageName("");
		setSourceKind("brew");
		setPackageVersion("latest");
	}

	return (
		<section className="resource-editor">
			<header>
				<div>
					<p className="eyebrow">Desired state</p>
					<h2>리소스 편집</h2>
				</div>
				<div className="segmented">
					<button
						className={mode === "package" ? "active" : ""}
						type="button"
						onClick={() => setMode("package")}
					>
						Package
					</button>
					<button
						className={mode === "dotfile" ? "active" : ""}
						type="button"
						onClick={() => {
							resetPackageEdit();
							setMode("dotfile");
						}}
					>
						Dotfile
					</button>
				</div>
			</header>
			{mode === "package" ? (
				<form className="editor-form" onSubmit={handlePackageSubmit}>
					<label>
						<span>Package name</span>
						<input
							aria-label="Package name"
							value={packageName}
							onChange={(event) => setPackageName(event.target.value)}
						/>
					</label>
					<label>
						<span>Source</span>
						<select
							aria-label="Package source"
							value={sourceKind}
							onChange={(event) =>
								setSourceKind(parsePackageSource(event.target.value))
							}
						>
							<option value="brew">brew supported</option>
							<option value="apt">apt unsupported</option>
							<option value="mise">mise unsupported</option>
						</select>
					</label>
					<label>
						<span>Package version</span>
						<input
							aria-label="Package version"
							value={packageVersion}
							onChange={(event) => setPackageVersion(event.target.value)}
						/>
					</label>
					<button className="primary-button" type="submit" disabled={isSaving}>
						Save package
					</button>
					{editingResourceId === "" ? null : (
						<button
							className="secondary-button"
							type="button"
							disabled={isSaving}
							onClick={resetPackageEdit}
						>
							Cancel edit
						</button>
					)}
					<PackageResourceList
						resources={resources}
						isSaving={isSaving}
						onDelete={handleDeletePackage}
						onEdit={startPackageEdit}
					/>
				</form>
			) : (
				<form className="editor-form" onSubmit={handleDotfileSubmit}>
					<label>
						<span>Dotfile path</span>
						<input
							aria-label="Dotfile path"
							value={dotfilePath}
							onChange={(event) => setDotfilePath(event.target.value)}
						/>
					</label>
					<label>
						<span>Dotfile content</span>
						<textarea
							aria-label="Dotfile content"
							value={dotfileContent}
							onChange={(event) => setDotfileContent(event.target.value)}
						/>
					</label>
					<button className="primary-button" type="submit" disabled={isSaving}>
						Save dotfile
					</button>
				</form>
			)}
			{message === "" ? null : <p className="editor-message">{message}</p>}
		</section>
	);
}

function parsePackageSource(value: string): "brew" | "apt" | "mise" {
	if (value === "apt" || value === "mise") {
		return value;
	}
	return "brew";
}
