// Bun text imports (`with { type: "text" }`) of collector scripts.
declare module "*.sh" {
	const text: string;
	export default text;
}
declare module "*.ps1" {
	const text: string;
	export default text;
}
