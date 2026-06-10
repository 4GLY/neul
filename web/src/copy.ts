export const englishAllowlist = [
	"CLI commands",
	"package names",
	"API paths",
	"protocol fields",
	"OS names",
	"semantic status enum values",
] as const;

export const dashboardCopy = {
	pageEyebrow: "개요",
	pageTitle: "머신",
	pageDescription:
		"개발 머신의 desired state, drift, reconcile 상태를 한 화면에서 확인합니다.",
	showLedger: "ledger 보기",
	showDashboard: "dashboard 보기",
	reconcileNow: "지금 reconcile",
	reconciling: "reconcile 중",
	metrics: {
		machines: "머신",
		healthy: "정상",
		drifted: "drift 있음",
		pendingChanges: "대기 중 변경",
		lastReconcile: "최근 reconcile",
	},
	emptyState: {
		title: "첫 머신을 등록하세요",
		body: "packaged neul client에서 등록 명령을 한 번 실행하면 agent 연결을 확인합니다.",
		action: "첫 머신 등록",
	},
} as const;

export const onboardingCopy = {
	title: "머신 등록",
	intro: "등록 명령을 만든 뒤 새 머신에서 실행하세요.",
	createInvite: "등록 명령 만들기",
	commandReady: "명령 실행 대기 중",
	checkingAgent: "agent 연결 확인 중",
	agentNotResponding: "agent 응답 없음",
	connected: "연결됨",
	expired: "등록 시간이 만료되었습니다.",
	used: "이미 사용된 등록 명령입니다.",
	cancelled: "등록이 취소되었습니다.",
	retry: "다시 만들기",
	installOptions: [
		"macOS: Homebrew tap 또는 signed .pkg",
		"Linux: Debian/Ubuntu .deb 또는 tarball",
	],
	checkoutHint: "Run with packaged neul client:",
	commandTemplate: "neul enroll --server <origin>",
	security: {
		pairTokenKind: "bearer",
		neverStorePairTokenIn: [
			"general URL query strings outside enrollment handoff",
			"document.title",
			"browser history",
		],
		allowedPairTokenHandoffs: [
			"127.0.0.1 local callback",
			"neul:// enrollment deep link",
			"fallback/debug command",
		],
	},
} as const;

export const filtersCopy = {
	searchMachines: "머신 검색",
	allStatus: "모든 상태",
	allOs: "모든 OS",
	filters: "필터",
} as const;

export const inspectorCopy = {
	tabs: {
		status: "상태",
		changes: "변경",
		config: "설정",
		logs: "로그",
	},
	repairDrift: "drift 복구",
	viewDiff: "diff 보기",
	openLogs: "로그 열기",
} as const;

export const ledgerCopy = {
	title: "desired vs live",
	viewHistory: "히스토리 보기",
	resource: "리소스",
	desired: "desired",
	selectedMachine: "선택한 머신",
} as const;

export const resourceEditorCopy = {
	title: "desired state 편집",
	addPackage: "package 추가",
	addDotfile: "dotfile 추가",
	save: "저장",
	cancel: "취소",
	unsupportedAdapter: "아직 agent adapter가 지원되지 않습니다.",
	pathNotAllowed: "허용된 HOME 경로만 사용할 수 있습니다.",
} as const;

export const disabledSecretsCopy = {
	label: "Secrets",
	status: "MVP에서는 비활성화됨",
	message: "secret 생성과 회전은 package/dotfile loop 이후에 추가합니다.",
} as const;

export const statusCopy = {
	healthy: "정상",
	drifted: "drift 있음",
	pending: "대기 중",
	offline: "오프라인",
	applied: "적용됨",
	blocked: "차단됨",
	unknown: "알 수 없음",
} as const;

export const copy = {
	dashboard: dashboardCopy,
	onboarding: onboardingCopy,
	filters: filtersCopy,
	inspector: inspectorCopy,
	ledger: ledgerCopy,
	resourceEditor: resourceEditorCopy,
	disabledSecrets: disabledSecretsCopy,
	status: statusCopy,
	englishAllowlist,
} as const;
