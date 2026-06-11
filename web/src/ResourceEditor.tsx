import type { FormEvent, ReactElement } from "react";
import { useState } from "react";
import {
	createPackageResource,
	deleteResource,
	OwnerSessionRequiredError,
	updatePackageResource,
} from "./api";
import type { ApiResource } from "./apiTypes";
import { DotfileResourceEditor } from "./DotfileResourceEditor";
import { PackageResourceList } from "./PackageResourceList";
import type { ResourceRow } from "./types";

type ResourceEditorProps = {
	readonly onOwnerSessionRequired?: () => void;
	readonly onSaved: () => void;
	readonly resources?: readonly ApiResource[];
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
	const [message, setMessage] = useState("");
	const [isSaving, setIsSaving] = useState(false);
	const packageResources = resources.flatMap((resource) =>
		toPackageResourceRow(resource),
	);

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
							onInput={(event) => setPackageName(event.currentTarget.value)}
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
							onInput={(event) => setPackageVersion(event.currentTarget.value)}
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
						resources={packageResources}
						isSaving={isSaving}
						onDelete={handleDeletePackage}
						onEdit={startPackageEdit}
					/>
				</form>
			) : (
				<DotfileResourceEditor
					isSaving={isSaving}
					resources={resources}
					onMessageChange={setMessage}
					onSaved={onSaved}
					onSavingChange={setIsSaving}
					{...(onOwnerSessionRequired === undefined
						? {}
						: { onOwnerSessionRequired })}
				/>
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

function toPackageResourceRow(resource: ApiResource): readonly ResourceRow[] {
	if (resource.kind !== "package") {
		return [];
	}
	const sourceKind = parsePackageSource(
		String(resource.spec.sourceKind ?? "brew"),
	);
	const desired =
		typeof resource.spec.desiredVersion === "string"
			? resource.spec.desiredVersion
			: `v${resource.desiredVersion}`;
	return [
		{
			desired,
			group: "패키지",
			id: resource.id,
			kind: "package",
			name: resource.name,
			sourceKind,
		},
	];
}
