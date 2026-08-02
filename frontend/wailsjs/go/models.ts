export namespace agent {
	
	export class AbortRequest {
	    taskId: string;
	    sessionId: string;
	    runId: string;
	
	    static createFrom(source: any = {}) {
	        return new AbortRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.taskId = source["taskId"];
	        this.sessionId = source["sessionId"];
	        this.runId = source["runId"];
	    }
	}
	export class AttachmentUpload {
	    name: string;
	    mimeType: string;
	    dataBase64: string;
	
	    static createFrom(source: any = {}) {
	        return new AttachmentUpload(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.mimeType = source["mimeType"];
	        this.dataBase64 = source["dataBase64"];
	    }
	}
	export class CommandRequest {
	    taskId: string;
	    sessionId: string;
	    type: string;
	    payload: Record<string, any>;
	
	    static createFrom(source: any = {}) {
	        return new CommandRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.taskId = source["taskId"];
	        this.sessionId = source["sessionId"];
	        this.type = source["type"];
	        this.payload = source["payload"];
	    }
	}
	export class CreateSessionRequest {
	    taskId: string;
	    title: string;
	    mode: string;
	    model: string;
	    thinkingLevel: string;
	    resourcePolicy: string;
	
	    static createFrom(source: any = {}) {
	        return new CreateSessionRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.taskId = source["taskId"];
	        this.title = source["title"];
	        this.mode = source["mode"];
	        this.model = source["model"];
	        this.thinkingLevel = source["thinkingLevel"];
	        this.resourcePolicy = source["resourcePolicy"];
	    }
	}
	export class HistoryPage {
	    messages: storage.AgentMessageRecord[];
	    nextCursor?: string;
	    hasMore: boolean;
	
	    static createFrom(source: any = {}) {
	        return new HistoryPage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.messages = this.convertValues(source["messages"], storage.AgentMessageRecord);
	        this.nextCursor = source["nextCursor"];
	        this.hasMore = source["hasMore"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class HistoryPageRequest {
	    taskId: string;
	    sessionId: string;
	    cursor: string;
	    limit: number;
	
	    static createFrom(source: any = {}) {
	        return new HistoryPageRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.taskId = source["taskId"];
	        this.sessionId = source["sessionId"];
	        this.cursor = source["cursor"];
	        this.limit = source["limit"];
	    }
	}
	export class ImportAttachmentsRequest {
	    taskId: string;
	    files: AttachmentUpload[];
	
	    static createFrom(source: any = {}) {
	        return new ImportAttachmentsRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.taskId = source["taskId"];
	        this.files = this.convertValues(source["files"], AttachmentUpload);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PermissionRequest {
	    id: string;
	    taskId: string;
	    sessionId: string;
	    runId: string;
	    toolCallId: string;
	    toolName: string;
	    capability: string;
	    target: string;
	    normalizedTarget?: string;
	    subject: string;
	    riskLevel: string;
	    state: string;
	    requestedAt: string;
	    expiresAt?: string;
	    resolvedAt?: string;
	    resolvedBy?: string;
	    decisionScope?: string;
	    reason?: string;
	    allowedScopes: string[];
	
	    static createFrom(source: any = {}) {
	        return new PermissionRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.taskId = source["taskId"];
	        this.sessionId = source["sessionId"];
	        this.runId = source["runId"];
	        this.toolCallId = source["toolCallId"];
	        this.toolName = source["toolName"];
	        this.capability = source["capability"];
	        this.target = source["target"];
	        this.normalizedTarget = source["normalizedTarget"];
	        this.subject = source["subject"];
	        this.riskLevel = source["riskLevel"];
	        this.state = source["state"];
	        this.requestedAt = source["requestedAt"];
	        this.expiresAt = source["expiresAt"];
	        this.resolvedAt = source["resolvedAt"];
	        this.resolvedBy = source["resolvedBy"];
	        this.decisionScope = source["decisionScope"];
	        this.reason = source["reason"];
	        this.allowedScopes = source["allowedScopes"];
	    }
	}
	export class PromptRequest {
	    taskId: string;
	    sessionId: string;
	    message: string;
	    resourceIds: string[];
	
	    static createFrom(source: any = {}) {
	        return new PromptRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.taskId = source["taskId"];
	        this.sessionId = source["sessionId"];
	        this.message = source["message"];
	        this.resourceIds = source["resourceIds"];
	    }
	}
	export class RemoveReferenceRequest {
	    taskId: string;
	    sessionId: string;
	    messageId: string;
	    resourceId: string;
	
	    static createFrom(source: any = {}) {
	        return new RemoveReferenceRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.taskId = source["taskId"];
	        this.sessionId = source["sessionId"];
	        this.messageId = source["messageId"];
	        this.resourceId = source["resourceId"];
	    }
	}
	export class ResolvePermissionRequest {
	    taskId: string;
	    sessionId: string;
	    requestId: string;
	    decision: string;
	    scope: string;
	
	    static createFrom(source: any = {}) {
	        return new ResolvePermissionRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.taskId = source["taskId"];
	        this.sessionId = source["sessionId"];
	        this.requestId = source["requestId"];
	        this.decision = source["decision"];
	        this.scope = source["scope"];
	    }
	}
	export class ResourceDescriptor {
	    id: string;
	    taskId: string;
	    targetType: string;
	    kind: string;
	    sourceType?: string;
	    logicalPath: string;
	    mimeType?: string;
	    byteSize: number;
	    sha256: string;
	    immutable: boolean;
	    readable: boolean;
	    createdAt: string;
	    updatedAt?: string;
	    proposalState?: string;
	
	    static createFrom(source: any = {}) {
	        return new ResourceDescriptor(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.taskId = source["taskId"];
	        this.targetType = source["targetType"];
	        this.kind = source["kind"];
	        this.sourceType = source["sourceType"];
	        this.logicalPath = source["logicalPath"];
	        this.mimeType = source["mimeType"];
	        this.byteSize = source["byteSize"];
	        this.sha256 = source["sha256"];
	        this.immutable = source["immutable"];
	        this.readable = source["readable"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	        this.proposalState = source["proposalState"];
	    }
	}
	export class ResourcePreviewRequest {
	    taskId: string;
	    resourceId: string;
	
	    static createFrom(source: any = {}) {
	        return new ResourcePreviewRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.taskId = source["taskId"];
	        this.resourceId = source["resourceId"];
	    }
	}
	export class ResourceSearchRequest {
	    taskId: string;
	    query: string;
	    limit: number;
	
	    static createFrom(source: any = {}) {
	        return new ResourceSearchRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.taskId = source["taskId"];
	        this.query = source["query"];
	        this.limit = source["limit"];
	    }
	}
	export class RevokePermissionGrantRequest {
	    taskId: string;
	    grantId: string;
	
	    static createFrom(source: any = {}) {
	        return new RevokePermissionGrantRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.taskId = source["taskId"];
	        this.grantId = source["grantId"];
	    }
	}
	export class SessionRequest {
	    taskId: string;
	    sessionId: string;
	
	    static createFrom(source: any = {}) {
	        return new SessionRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.taskId = source["taskId"];
	        this.sessionId = source["sessionId"];
	    }
	}
	export class ToolOutput {
	    reference: string;
	    content: string;
	    byteSize: number;
	    truncated: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ToolOutput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.reference = source["reference"];
	        this.content = source["content"];
	        this.byteSize = source["byteSize"];
	        this.truncated = source["truncated"];
	    }
	}
	export class ToolOutputRequest {
	    taskId: string;
	    toolCallId: string;
	
	    static createFrom(source: any = {}) {
	        return new ToolOutputRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.taskId = source["taskId"];
	        this.toolCallId = source["toolCallId"];
	    }
	}

}

export namespace bridge {
	
	export class HistoryMessageView {
	    role: string;
	    content: string;
	    detail?: string;
	    turn?: number;
	    model?: string;
	    createdAt?: number;
	    level?: string;
	
	    static createFrom(source: any = {}) {
	        return new HistoryMessageView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.role = source["role"];
	        this.content = source["content"];
	        this.detail = source["detail"];
	        this.turn = source["turn"];
	        this.model = source["model"];
	        this.createdAt = source["createdAt"];
	        this.level = source["level"];
	    }
	}
	export class HistoryPageView {
	    messages: HistoryMessageView[];
	    startTurn: number;
	    endTurn: number;
	    totalTurns: number;
	    hasOlder: boolean;
	
	    static createFrom(source: any = {}) {
	        return new HistoryPageView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.messages = this.convertValues(source["messages"], HistoryMessageView);
	        this.startTurn = source["startTurn"];
	        this.endTurn = source["endTurn"];
	        this.totalTurns = source["totalTurns"];
	        this.hasOlder = source["hasOlder"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class MetaView {
	    tabId: string;
	    model: string;
	    effort: string;
	    tokenMode: string;
	    mode: string;
	    toolApprovalMode: string;
	    sessionPath: string;
	    label: string;
	    ready: boolean;
	    eventChannel: string;
	    cwd: string;
	    workspaceRoot?: string;
	    workspaceName?: string;
	    goal?: string;
	    goalStatus?: string;
	
	    static createFrom(source: any = {}) {
	        return new MetaView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tabId = source["tabId"];
	        this.model = source["model"];
	        this.effort = source["effort"];
	        this.tokenMode = source["tokenMode"];
	        this.mode = source["mode"];
	        this.toolApprovalMode = source["toolApprovalMode"];
	        this.sessionPath = source["sessionPath"];
	        this.label = source["label"];
	        this.ready = source["ready"];
	        this.eventChannel = source["eventChannel"];
	        this.cwd = source["cwd"];
	        this.workspaceRoot = source["workspaceRoot"];
	        this.workspaceName = source["workspaceName"];
	        this.goal = source["goal"];
	        this.goalStatus = source["goalStatus"];
	    }
	}
	export class ModelInfoView {
	    name: string;
	    ref: string;
	    provider: string;
	    kind?: string;
	    current?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ModelInfoView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.ref = source["ref"];
	        this.provider = source["provider"];
	        this.kind = source["kind"];
	        this.current = source["current"];
	    }
	}
	export class SessionMetaView {
	    path: string;
	    preview: string;
	    title?: string;
	    turns: number;
	    createdAt: number;
	    lastActivityAt: number;
	    modTime: number;
	    current: boolean;
	    open: boolean;
	    scope?: string;
	    workspaceRoot?: string;
	    topicId?: string;
	    topicTitle?: string;
	
	    static createFrom(source: any = {}) {
	        return new SessionMetaView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.preview = source["preview"];
	        this.title = source["title"];
	        this.turns = source["turns"];
	        this.createdAt = source["createdAt"];
	        this.lastActivityAt = source["lastActivityAt"];
	        this.modTime = source["modTime"];
	        this.current = source["current"];
	        this.open = source["open"];
	        this.scope = source["scope"];
	        this.workspaceRoot = source["workspaceRoot"];
	        this.topicId = source["topicId"];
	        this.topicTitle = source["topicTitle"];
	    }
	}
	export class TabView {
	    id: string;
	    scope: string;
	    workspaceRoot: string;
	    workspaceName: string;
	    workspacePath?: string;
	    gitBranch?: string;
	    topicId: string;
	    topicTitle: string;
	    sessionPath?: string;
	    label: string;
	    ready: boolean;
	    running: boolean;
	    pendingPrompt?: boolean;
	    cancellable: boolean;
	    mode: string;
	    collaborationMode: string;
	    toolApprovalMode: string;
	    tokenMode: string;
	    goal?: string;
	    goalStatus?: string;
	    recovered?: boolean;
	    startupErr?: string;
	    active: boolean;
	    cwd: string;
	
	    static createFrom(source: any = {}) {
	        return new TabView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.scope = source["scope"];
	        this.workspaceRoot = source["workspaceRoot"];
	        this.workspaceName = source["workspaceName"];
	        this.workspacePath = source["workspacePath"];
	        this.gitBranch = source["gitBranch"];
	        this.topicId = source["topicId"];
	        this.topicTitle = source["topicTitle"];
	        this.sessionPath = source["sessionPath"];
	        this.label = source["label"];
	        this.ready = source["ready"];
	        this.running = source["running"];
	        this.pendingPrompt = source["pendingPrompt"];
	        this.cancellable = source["cancellable"];
	        this.mode = source["mode"];
	        this.collaborationMode = source["collaborationMode"];
	        this.toolApprovalMode = source["toolApprovalMode"];
	        this.tokenMode = source["tokenMode"];
	        this.goal = source["goal"];
	        this.goalStatus = source["goalStatus"];
	        this.recovered = source["recovered"];
	        this.startupErr = source["startupErr"];
	        this.active = source["active"];
	        this.cwd = source["cwd"];
	    }
	}

}

export namespace engine {
	
	export class CandidateAnalysis {
	    mode: string;
	    title: string;
	    summaryMarkdown: string;
	    keyPoints: string[];
	    openQuestions: string[];
	    analyzedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new CandidateAnalysis(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.title = source["title"];
	        this.summaryMarkdown = source["summaryMarkdown"];
	        this.keyPoints = source["keyPoints"];
	        this.openQuestions = source["openQuestions"];
	        this.analyzedAt = source["analyzedAt"];
	    }
	}
	export class DailyReportGeneratedBlocker {
	    projectNo: string;
	    projectName: string;
	    issue: string;
	    level: string;
	    impact: string;
	    helpTarget: string;
	    waitDuration: string;
	    escalate: string;
	
	    static createFrom(source: any = {}) {
	        return new DailyReportGeneratedBlocker(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.projectNo = source["projectNo"];
	        this.projectName = source["projectName"];
	        this.issue = source["issue"];
	        this.level = source["level"];
	        this.impact = source["impact"];
	        this.helpTarget = source["helpTarget"];
	        this.waitDuration = source["waitDuration"];
	        this.escalate = source["escalate"];
	    }
	}
	export class DailyReportGeneratedNextAction {
	    projectNo: string;
	    projectName: string;
	    goal: string;
	    deadline: string;
	    inferred: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DailyReportGeneratedNextAction(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.projectNo = source["projectNo"];
	        this.projectName = source["projectName"];
	        this.goal = source["goal"];
	        this.deadline = source["deadline"];
	        this.inferred = source["inferred"];
	    }
	}
	export class DailyReportGeneratedResult {
	    projectNo: string;
	    projectName: string;
	    task: string;
	    status: string;
	    progress: string;
	    evidence: string[];
	
	    static createFrom(source: any = {}) {
	        return new DailyReportGeneratedResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.projectNo = source["projectNo"];
	        this.projectName = source["projectName"];
	        this.task = source["task"];
	        this.status = source["status"];
	        this.progress = source["progress"];
	        this.evidence = source["evidence"];
	    }
	}
	export class DailyReportGeneratedReview {
	    scene: string;
	    cause: string;
	    action: string;
	    validation: string;
	    teamRisk: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DailyReportGeneratedReview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.scene = source["scene"];
	        this.cause = source["cause"];
	        this.action = source["action"];
	        this.validation = source["validation"];
	        this.teamRisk = source["teamRisk"];
	    }
	}
	export class DailyReportGenerationWorkflowTask {
	    taskId: string;
	    title: string;
	    projectName: string;
	    status: string;
	    developmentState: string;
	    summary: string;
	    developmentResult: string;
	    reviewNote: string;
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new DailyReportGenerationWorkflowTask(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.taskId = source["taskId"];
	        this.title = source["title"];
	        this.projectName = source["projectName"];
	        this.status = source["status"];
	        this.developmentState = source["developmentState"];
	        this.summary = source["summary"];
	        this.developmentResult = source["developmentResult"];
	        this.reviewNote = source["reviewNote"];
	        this.updatedAt = source["updatedAt"];
	    }
	}
	export class DailyReportGenerationProject {
	    projectNo: string;
	    projectName: string;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new DailyReportGenerationProject(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.projectNo = source["projectNo"];
	        this.projectName = source["projectName"];
	        this.path = source["path"];
	    }
	}
	export class DailyReportGenerationInput {
	    requestId: string;
	    reportDate: string;
	    organization: string;
	    level: string;
	    role: string;
	    engine: string;
	    customInstructions: string;
	    gitAuthor: string;
	    includeUncommitted: boolean;
	    projects: DailyReportGenerationProject[];
	    manualDescription: string;
	    workflowTasks: DailyReportGenerationWorkflowTask[];
	
	    static createFrom(source: any = {}) {
	        return new DailyReportGenerationInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.requestId = source["requestId"];
	        this.reportDate = source["reportDate"];
	        this.organization = source["organization"];
	        this.level = source["level"];
	        this.role = source["role"];
	        this.engine = source["engine"];
	        this.customInstructions = source["customInstructions"];
	        this.gitAuthor = source["gitAuthor"];
	        this.includeUncommitted = source["includeUncommitted"];
	        this.projects = this.convertValues(source["projects"], DailyReportGenerationProject);
	        this.manualDescription = source["manualDescription"];
	        this.workflowTasks = this.convertValues(source["workflowTasks"], DailyReportGenerationWorkflowTask);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class DailyReportGenerationResult {
	    reportDate: string;
	    results: DailyReportGeneratedResult[];
	    blockers: DailyReportGeneratedBlocker[];
	    reviews: DailyReportGeneratedReview[];
	    nextActions: DailyReportGeneratedNextAction[];
	
	    static createFrom(source: any = {}) {
	        return new DailyReportGenerationResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.reportDate = source["reportDate"];
	        this.results = this.convertValues(source["results"], DailyReportGeneratedResult);
	        this.blockers = this.convertValues(source["blockers"], DailyReportGeneratedBlocker);
	        this.reviews = this.convertValues(source["reviews"], DailyReportGeneratedReview);
	        this.nextActions = this.convertValues(source["nextActions"], DailyReportGeneratedNextAction);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class PISettings {
	    model: string;
	    thinkingEffort: string;
	    timeoutMinutes: number;
	    resourcePolicy: string;
	
	    static createFrom(source: any = {}) {
	        return new PISettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.model = source["model"];
	        this.thinkingEffort = source["thinkingEffort"];
	        this.timeoutMinutes = source["timeoutMinutes"];
	        this.resourcePolicy = source["resourcePolicy"];
	    }
	}
	export class RequirementAnalysisPolicy {
	    allowAssumption: boolean;
	    requireSourceForFact: boolean;
	    allowCodeWrite: boolean;
	    allowProjectRead: boolean;
	    askWhenAmbiguous: boolean;
	    forceProceedRequested: boolean;
	
	    static createFrom(source: any = {}) {
	        return new RequirementAnalysisPolicy(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.allowAssumption = source["allowAssumption"];
	        this.requireSourceForFact = source["requireSourceForFact"];
	        this.allowCodeWrite = source["allowCodeWrite"];
	        this.allowProjectRead = source["allowProjectRead"];
	        this.askWhenAmbiguous = source["askWhenAmbiguous"];
	        this.forceProceedRequested = source["forceProceedRequested"];
	    }
	}
	export class RequirementExistingRevision {
	    version: number;
	    objective: string;
	    scope: string[];
	    outOfScope: string[];
	    acceptanceCriteria: string[];
	    risks: string[];
	    content: string;
	
	    static createFrom(source: any = {}) {
	        return new RequirementExistingRevision(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.objective = source["objective"];
	        this.scope = source["scope"];
	        this.outOfScope = source["outOfScope"];
	        this.acceptanceCriteria = source["acceptanceCriteria"];
	        this.risks = source["risks"];
	        this.content = source["content"];
	    }
	}
	export class RequirementProjectEvidence {
	    path: string;
	    summary: string;
	    lineRange?: string;
	
	    static createFrom(source: any = {}) {
	        return new RequirementProjectEvidence(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.summary = source["summary"];
	        this.lineRange = source["lineRange"];
	    }
	}
	export class RequirementQuestion {
	    id: string;
	    round?: number;
	    category: string;
	    severity: string;
	    question: string;
	    reason: string;
	    sourceFragmentIds?: string[];
	    projectEvidence?: RequirementProjectEvidence[];
	    answerType: string;
	    options: string[];
	    allowCustomAnswer: boolean;
	    status?: string;
	    answer?: string;
	
	    static createFrom(source: any = {}) {
	        return new RequirementQuestion(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.round = source["round"];
	        this.category = source["category"];
	        this.severity = source["severity"];
	        this.question = source["question"];
	        this.reason = source["reason"];
	        this.sourceFragmentIds = source["sourceFragmentIds"];
	        this.projectEvidence = this.convertValues(source["projectEvidence"], RequirementProjectEvidence);
	        this.answerType = source["answerType"];
	        this.options = source["options"];
	        this.allowCustomAnswer = source["allowCustomAnswer"];
	        this.status = source["status"];
	        this.answer = source["answer"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class RequirementPreviousAnswer {
	    questionId: string;
	    question: string;
	    answer: string;
	    status: string;
	    answeredBy: string;
	
	    static createFrom(source: any = {}) {
	        return new RequirementPreviousAnswer(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.questionId = source["questionId"];
	        this.question = source["question"];
	        this.answer = source["answer"];
	        this.status = source["status"];
	        this.answeredBy = source["answeredBy"];
	    }
	}
	export class RequirementMaterialFragment {
	    fragmentId: string;
	    content: string;
	    sourceLocation: string;
	
	    static createFrom(source: any = {}) {
	        return new RequirementMaterialFragment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.fragmentId = source["fragmentId"];
	        this.content = source["content"];
	        this.sourceLocation = source["sourceLocation"];
	    }
	}
	export class RequirementMaterial {
	    materialId: string;
	    type: string;
	    relationship: string;
	    title: string;
	    fragments: RequirementMaterialFragment[];
	
	    static createFrom(source: any = {}) {
	        return new RequirementMaterial(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.materialId = source["materialId"];
	        this.type = source["type"];
	        this.relationship = source["relationship"];
	        this.title = source["title"];
	        this.fragments = this.convertValues(source["fragments"], RequirementMaterialFragment);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class RequirementProject {
	    name: string;
	    localPath: string;
	
	    static createFrom(source: any = {}) {
	        return new RequirementProject(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.localPath = source["localPath"];
	    }
	}
	export class RequirementTask {
	    id: string;
	    title: string;
	    originalDescription: string;
	    currentStatus: string;
	
	    static createFrom(source: any = {}) {
	        return new RequirementTask(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.originalDescription = source["originalDescription"];
	        this.currentStatus = source["currentStatus"];
	    }
	}
	export class RequirementAnalysisInput {
	    task: RequirementTask;
	    project: RequirementProject;
	    materials: RequirementMaterial[];
	    previousAnswers: RequirementPreviousAnswer[];
	    openQuestions: RequirementQuestion[];
	    existingRequirementRevision: RequirementExistingRevision;
	    analysisPolicy: RequirementAnalysisPolicy;
	    analyst: string;
	    round: number;
	    mode: string;
	    focus: string;
	
	    static createFrom(source: any = {}) {
	        return new RequirementAnalysisInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.task = this.convertValues(source["task"], RequirementTask);
	        this.project = this.convertValues(source["project"], RequirementProject);
	        this.materials = this.convertValues(source["materials"], RequirementMaterial);
	        this.previousAnswers = this.convertValues(source["previousAnswers"], RequirementPreviousAnswer);
	        this.openQuestions = this.convertValues(source["openQuestions"], RequirementQuestion);
	        this.existingRequirementRevision = this.convertValues(source["existingRequirementRevision"], RequirementExistingRevision);
	        this.analysisPolicy = this.convertValues(source["analysisPolicy"], RequirementAnalysisPolicy);
	        this.analyst = source["analyst"];
	        this.round = source["round"];
	        this.mode = source["mode"];
	        this.focus = source["focus"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class RequirementDraftUpdates {
	    objective?: string;
	    scope: string[];
	    outOfScope: string[];
	    acceptanceCriteria: string[];
	    constraints: string[];
	
	    static createFrom(source: any = {}) {
	        return new RequirementDraftUpdates(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.objective = source["objective"];
	        this.scope = source["scope"];
	        this.outOfScope = source["outOfScope"];
	        this.acceptanceCriteria = source["acceptanceCriteria"];
	        this.constraints = source["constraints"];
	    }
	}
	export class RequirementConflict {
	    id: string;
	    description: string;
	    sourceA: string;
	    sourceB: string;
	
	    static createFrom(source: any = {}) {
	        return new RequirementConflict(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.description = source["description"];
	        this.sourceA = source["sourceA"];
	        this.sourceB = source["sourceB"];
	    }
	}
	export class RequirementProjectObservation {
	    id: string;
	    content: string;
	    filePath: string;
	    lineRange?: string;
	
	    static createFrom(source: any = {}) {
	        return new RequirementProjectObservation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.content = source["content"];
	        this.filePath = source["filePath"];
	        this.lineRange = source["lineRange"];
	    }
	}
	export class RequirementFact {
	    id: string;
	    content: string;
	    sourceFragmentIds: string[];
	
	    static createFrom(source: any = {}) {
	        return new RequirementFact(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.content = source["content"];
	        this.sourceFragmentIds = source["sourceFragmentIds"];
	    }
	}
	export class RequirementAnalysisResult {
	    analysisId: string;
	    round: number;
	    confirmedFacts: RequirementFact[];
	    projectObservations: RequirementProjectObservation[];
	    questions: RequirementQuestion[];
	    conflicts: RequirementConflict[];
	    draftUpdates: RequirementDraftUpdates;
	    analysisStatus: string;
	    recommendedAction: string;
	    reason: string;
	    analyzedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new RequirementAnalysisResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.analysisId = source["analysisId"];
	        this.round = source["round"];
	        this.confirmedFacts = this.convertValues(source["confirmedFacts"], RequirementFact);
	        this.projectObservations = this.convertValues(source["projectObservations"], RequirementProjectObservation);
	        this.questions = this.convertValues(source["questions"], RequirementQuestion);
	        this.conflicts = this.convertValues(source["conflicts"], RequirementConflict);
	        this.draftUpdates = this.convertValues(source["draftUpdates"], RequirementDraftUpdates);
	        this.analysisStatus = source["analysisStatus"];
	        this.recommendedAction = source["recommendedAction"];
	        this.reason = source["reason"];
	        this.analyzedAt = source["analyzedAt"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	
	
	
	
	
	
	
	
	
	export class Status {
	    id: string;
	    label: string;
	    configured: boolean;
	    requirementAnalysis: boolean;
	    development: boolean;
	    description: string;
	    commandPath: string;
	    version: string;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.configured = source["configured"];
	        this.requirementAnalysis = source["requirementAnalysis"];
	        this.development = source["development"];
	        this.description = source["description"];
	        this.commandPath = source["commandPath"];
	        this.version = source["version"];
	    }
	}

}

export namespace execution {
	
	export class StopRequest {
	    taskId: string;
	    sessionId: string;
	    runId: string;
	    toolCallId: string;
	
	    static createFrom(source: any = {}) {
	        return new StopRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.taskId = source["taskId"];
	        this.sessionId = source["sessionId"];
	        this.runId = source["runId"];
	        this.toolCallId = source["toolCallId"];
	    }
	}

}

export namespace gitrepo {
	
	export class BindRequest {
	    taskId: string;
	    sourcePath: string;
	
	    static createFrom(source: any = {}) {
	        return new BindRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.taskId = source["taskId"];
	        this.sourcePath = source["sourcePath"];
	    }
	}
	export class ChangedFile {
	    path: string;
	    originalPath?: string;
	    status: string;
	    indexStatus: string;
	    worktreeStatus: string;
	    staged: boolean;
	    unstaged: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ChangedFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.originalPath = source["originalPath"];
	        this.status = source["status"];
	        this.indexStatus = source["indexStatus"];
	        this.worktreeStatus = source["worktreeStatus"];
	        this.staged = source["staged"];
	        this.unstaged = source["unstaged"];
	    }
	}
	export class CleanupRequest {
	    taskId: string;
	    confirmed: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CleanupRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.taskId = source["taskId"];
	        this.confirmed = source["confirmed"];
	    }
	}
	export class CommitRequest {
	    taskId: string;
	    message: string;
	    expectedSnapshot: string;
	    confirmed: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CommitRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.taskId = source["taskId"];
	        this.message = source["message"];
	        this.expectedSnapshot = source["expectedSnapshot"];
	        this.confirmed = source["confirmed"];
	    }
	}
	export class StatusView {
	    bound: boolean;
	    binding: storage.GitBindingRecord;
	    remoteUrl?: string;
	    head?: string;
	    snapshot?: string;
	    files: ChangedFile[];
	    aheadOfBaseline: number;
	    behindBaseline: number;
	    errorMessage?: string;
	
	    static createFrom(source: any = {}) {
	        return new StatusView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.bound = source["bound"];
	        this.binding = this.convertValues(source["binding"], storage.GitBindingRecord);
	        this.remoteUrl = source["remoteUrl"];
	        this.head = source["head"];
	        this.snapshot = source["snapshot"];
	        this.files = this.convertValues(source["files"], ChangedFile);
	        this.aheadOfBaseline = source["aheadOfBaseline"];
	        this.behindBaseline = source["behindBaseline"];
	        this.errorMessage = source["errorMessage"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CommitResult {
	    commit: string;
	    status: StatusView;
	
	    static createFrom(source: any = {}) {
	        return new CommitResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.commit = source["commit"];
	        this.status = this.convertValues(source["status"], StatusView);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class FileDiffView {
	    taskId: string;
	    path: string;
	    status: string;
	    staged: string;
	    unstaged: string;
	    added: number;
	    removed: number;
	    binary: boolean;
	    truncated: boolean;
	    byteSize: number;
	
	    static createFrom(source: any = {}) {
	        return new FileDiffView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.taskId = source["taskId"];
	        this.path = source["path"];
	        this.status = source["status"];
	        this.staged = source["staged"];
	        this.unstaged = source["unstaged"];
	        this.added = source["added"];
	        this.removed = source["removed"];
	        this.binary = source["binary"];
	        this.truncated = source["truncated"];
	        this.byteSize = source["byteSize"];
	    }
	}
	export class RecoverRequest {
	    taskId: string;
	    confirmed: boolean;
	
	    static createFrom(source: any = {}) {
	        return new RecoverRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.taskId = source["taskId"];
	        this.confirmed = source["confirmed"];
	    }
	}

}

export namespace plane {
	
	export class CandidateReference {
	    externalId: string;
	    externalKey: string;
	    title: string;
	
	    static createFrom(source: any = {}) {
	        return new CandidateReference(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.externalId = source["externalId"];
	        this.externalKey = source["externalKey"];
	        this.title = source["title"];
	    }
	}
	export class Comment {
	    id: string;
	    bodyMarkdown: string;
	    actor: Person;
	    createdAt?: string;
	    updatedAt?: string;
	    editedAt?: string;
	
	    static createFrom(source: any = {}) {
	        return new Comment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.bodyMarkdown = source["bodyMarkdown"];
	        this.actor = this.convertValues(source["actor"], Person);
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	        this.editedAt = source["editedAt"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Person {
	    id: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new Person(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	    }
	}
	export class Candidate {
	    externalId: string;
	    externalKey: string;
	    title: string;
	    descriptionMarkdown: string;
	    sourceMarkdown: string;
	    priority: string;
	    stateName: string;
	    stateGroup: string;
	    labels: string[];
	    assignees: string[];
	    assigneeDetails: Person[];
	    comments: Comment[];
	    commentsSyncError?: string;
	    detailsLoaded: boolean;
	    parent?: CandidateReference;
	    createdAt?: string;
	    updatedAt?: string;
	
	    static createFrom(source: any = {}) {
	        return new Candidate(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.externalId = source["externalId"];
	        this.externalKey = source["externalKey"];
	        this.title = source["title"];
	        this.descriptionMarkdown = source["descriptionMarkdown"];
	        this.sourceMarkdown = source["sourceMarkdown"];
	        this.priority = source["priority"];
	        this.stateName = source["stateName"];
	        this.stateGroup = source["stateGroup"];
	        this.labels = source["labels"];
	        this.assignees = source["assignees"];
	        this.assigneeDetails = this.convertValues(source["assigneeDetails"], Person);
	        this.comments = this.convertValues(source["comments"], Comment);
	        this.commentsSyncError = source["commentsSyncError"];
	        this.detailsLoaded = source["detailsLoaded"];
	        this.parent = this.convertValues(source["parent"], CandidateReference);
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	export class Project {
	    id: string;
	    name: string;
	    identifier: string;
	
	    static createFrom(source: any = {}) {
	        return new Project(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.identifier = source["identifier"];
	    }
	}
	export class ConnectionSetup {
	    baseUrl: string;
	    workspaceSlug: string;
	    projects: Project[];
	
	    static createFrom(source: any = {}) {
	        return new ConnectionSetup(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.baseUrl = source["baseUrl"];
	        this.workspaceSlug = source["workspaceSlug"];
	        this.projects = this.convertValues(source["projects"], Project);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ConnectionStatus {
	    connected: boolean;
	    itemCount: number;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new ConnectionStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.connected = source["connected"];
	        this.itemCount = source["itemCount"];
	        this.message = source["message"];
	    }
	}
	

}

export namespace report {
	
	export class SubmitResult {
	    id: any;
	    action: string;
	    message?: string;
	
	    static createFrom(source: any = {}) {
	        return new SubmitResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.action = source["action"];
	        this.message = source["message"];
	    }
	}

}

export namespace storage {
	
	export class AgentReferenceRecord {
	    taskId: string;
	    sessionId: string;
	    messageId: string;
	    resourceId: string;
	    targetType: string;
	    method: string;
	    position: number;
	    createdAt: string;
	    kind: string;
	    sourceType?: string;
	    logicalPath: string;
	    mimeType?: string;
	    byteSize?: number;
	    immutable: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AgentReferenceRecord(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.taskId = source["taskId"];
	        this.sessionId = source["sessionId"];
	        this.messageId = source["messageId"];
	        this.resourceId = source["resourceId"];
	        this.targetType = source["targetType"];
	        this.method = source["method"];
	        this.position = source["position"];
	        this.createdAt = source["createdAt"];
	        this.kind = source["kind"];
	        this.sourceType = source["sourceType"];
	        this.logicalPath = source["logicalPath"];
	        this.mimeType = source["mimeType"];
	        this.byteSize = source["byteSize"];
	        this.immutable = source["immutable"];
	    }
	}
	export class AgentMessageRecord {
	    id: string;
	    taskId: string;
	    sessionId: string;
	    runId?: string;
	    role: string;
	    kind: string;
	    status: string;
	    content?: string;
	    contentRef?: string;
	    sequence: number;
	    piEntryId?: string;
	    createdAt: string;
	    completedAt?: string;
	    references?: AgentReferenceRecord[];
	
	    static createFrom(source: any = {}) {
	        return new AgentMessageRecord(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.taskId = source["taskId"];
	        this.sessionId = source["sessionId"];
	        this.runId = source["runId"];
	        this.role = source["role"];
	        this.kind = source["kind"];
	        this.status = source["status"];
	        this.content = source["content"];
	        this.contentRef = source["contentRef"];
	        this.sequence = source["sequence"];
	        this.piEntryId = source["piEntryId"];
	        this.createdAt = source["createdAt"];
	        this.completedAt = source["completedAt"];
	        this.references = this.convertValues(source["references"], AgentReferenceRecord);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class AgentSessionRecord {
	    id: string;
	    taskId: string;
	    engine: string;
	    externalSessionPath?: string;
	    externalSessionId?: string;
	    title: string;
	    mode: string;
	    model?: string;
	    thinkingLevel?: string;
	    resourcePolicy: string;
	    state: string;
	    lastEntryId?: string;
	    lastSequence: number;
	    createdAt: string;
	    updatedAt: string;
	    lastActiveAt: string;
	    errorMessage?: string;
	
	    static createFrom(source: any = {}) {
	        return new AgentSessionRecord(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.taskId = source["taskId"];
	        this.engine = source["engine"];
	        this.externalSessionPath = source["externalSessionPath"];
	        this.externalSessionId = source["externalSessionId"];
	        this.title = source["title"];
	        this.mode = source["mode"];
	        this.model = source["model"];
	        this.thinkingLevel = source["thinkingLevel"];
	        this.resourcePolicy = source["resourcePolicy"];
	        this.state = source["state"];
	        this.lastEntryId = source["lastEntryId"];
	        this.lastSequence = source["lastSequence"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	        this.lastActiveAt = source["lastActiveAt"];
	        this.errorMessage = source["errorMessage"];
	    }
	}
	export class ExecutionRunRecord {
	    id: string;
	    taskId: string;
	    sessionId: string;
	    requirementRevision?: string;
	    gitBindingId?: string;
	    baselineCommit?: string;
	    mode: string;
	    state: string;
	    eventsPath: string;
	    stdoutPath: string;
	    stderrPath: string;
	    resultPath: string;
	    startedAt: string;
	    finishedAt?: string;
	    resultSummary?: string;
	    errorMessage?: string;
	
	    static createFrom(source: any = {}) {
	        return new ExecutionRunRecord(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.taskId = source["taskId"];
	        this.sessionId = source["sessionId"];
	        this.requirementRevision = source["requirementRevision"];
	        this.gitBindingId = source["gitBindingId"];
	        this.baselineCommit = source["baselineCommit"];
	        this.mode = source["mode"];
	        this.state = source["state"];
	        this.eventsPath = source["eventsPath"];
	        this.stdoutPath = source["stdoutPath"];
	        this.stderrPath = source["stderrPath"];
	        this.resultPath = source["resultPath"];
	        this.startedAt = source["startedAt"];
	        this.finishedAt = source["finishedAt"];
	        this.resultSummary = source["resultSummary"];
	        this.errorMessage = source["errorMessage"];
	    }
	}
	export class GitBindingRecord {
	    id: string;
	    taskId: string;
	    sourcePath: string;
	    sourceRealPath: string;
	    commonGitDir: string;
	    worktreePath?: string;
	    branch?: string;
	    baselineCommit: string;
	    sourceBranch?: string;
	    sourceDirtyAtBind: boolean;
	    state: string;
	    createdAt: string;
	    updatedAt: string;
	    errorMessage?: string;
	
	    static createFrom(source: any = {}) {
	        return new GitBindingRecord(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.taskId = source["taskId"];
	        this.sourcePath = source["sourcePath"];
	        this.sourceRealPath = source["sourceRealPath"];
	        this.commonGitDir = source["commonGitDir"];
	        this.worktreePath = source["worktreePath"];
	        this.branch = source["branch"];
	        this.baselineCommit = source["baselineCommit"];
	        this.sourceBranch = source["sourceBranch"];
	        this.sourceDirtyAtBind = source["sourceDirtyAtBind"];
	        this.state = source["state"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	        this.errorMessage = source["errorMessage"];
	    }
	}
	export class PermissionGrantRecord {
	    id: string;
	    taskId?: string;
	    sessionId?: string;
	    requestId?: string;
	    capability: string;
	    targetPattern: string;
	    scope: string;
	    decision: string;
	    riskCeiling: string;
	    createdAt: string;
	    expiresAt?: string;
	    consumedAt?: string;
	    revokedAt?: string;
	    createdBy: string;
	
	    static createFrom(source: any = {}) {
	        return new PermissionGrantRecord(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.taskId = source["taskId"];
	        this.sessionId = source["sessionId"];
	        this.requestId = source["requestId"];
	        this.capability = source["capability"];
	        this.targetPattern = source["targetPattern"];
	        this.scope = source["scope"];
	        this.decision = source["decision"];
	        this.riskCeiling = source["riskCeiling"];
	        this.createdAt = source["createdAt"];
	        this.expiresAt = source["expiresAt"];
	        this.consumedAt = source["consumedAt"];
	        this.revokedAt = source["revokedAt"];
	        this.createdBy = source["createdBy"];
	    }
	}
	export class TaskContextRootInfo {
	    path: string;
	    defaultPath: string;
	    custom: boolean;
	    available: boolean;
	    databaseSchemaVersion: number;
	    taskWorkspaceSchemaVersion: number;
	    workspaceCount: number;
	    workspaceErrorCount: number;
	
	    static createFrom(source: any = {}) {
	        return new TaskContextRootInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.defaultPath = source["defaultPath"];
	        this.custom = source["custom"];
	        this.available = source["available"];
	        this.databaseSchemaVersion = source["databaseSchemaVersion"];
	        this.taskWorkspaceSchemaVersion = source["taskWorkspaceSchemaVersion"];
	        this.workspaceCount = source["workspaceCount"];
	        this.workspaceErrorCount = source["workspaceErrorCount"];
	    }
	}
	export class TaskWorkspaceRecord {
	    taskId: string;
	    workspaceId: string;
	    rootPath: string;
	    schemaVersion: number;
	    manifestRevision: number;
	    state: string;
	    legacyContextPath?: string;
	    createdAt: string;
	    updatedAt: string;
	    lastReconciledAt?: string;
	    errorMessage?: string;
	
	    static createFrom(source: any = {}) {
	        return new TaskWorkspaceRecord(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.taskId = source["taskId"];
	        this.workspaceId = source["workspaceId"];
	        this.rootPath = source["rootPath"];
	        this.schemaVersion = source["schemaVersion"];
	        this.manifestRevision = source["manifestRevision"];
	        this.state = source["state"];
	        this.legacyContextPath = source["legacyContextPath"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	        this.lastReconciledAt = source["lastReconciledAt"];
	        this.errorMessage = source["errorMessage"];
	    }
	}
	export class ToolCallRecord {
	    id: string;
	    taskId: string;
	    sessionId: string;
	    runId: string;
	    externalToolCallId: string;
	    toolName: string;
	    capability: string;
	    target?: string;
	    riskLevel: string;
	    state: string;
	    argsJson?: string;
	    argsRef?: string;
	    outputSummary?: string;
	    outputRef?: string;
	    isError: boolean;
	    startedAt?: string;
	    finishedAt?: string;
	
	    static createFrom(source: any = {}) {
	        return new ToolCallRecord(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.taskId = source["taskId"];
	        this.sessionId = source["sessionId"];
	        this.runId = source["runId"];
	        this.externalToolCallId = source["externalToolCallId"];
	        this.toolName = source["toolName"];
	        this.capability = source["capability"];
	        this.target = source["target"];
	        this.riskLevel = source["riskLevel"];
	        this.state = source["state"];
	        this.argsJson = source["argsJson"];
	        this.argsRef = source["argsRef"];
	        this.outputSummary = source["outputSummary"];
	        this.outputRef = source["outputRef"];
	        this.isError = source["isError"];
	        this.startedAt = source["startedAt"];
	        this.finishedAt = source["finishedAt"];
	    }
	}

}

export namespace taskspace {
	
	export class FilePreview {
	    path: string;
	    name: string;
	    mimeType: string;
	    byteSize: number;
	    sha256: string;
	    kind: string;
	    content: string;
	    truncated: boolean;
	
	    static createFrom(source: any = {}) {
	        return new FilePreview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.name = source["name"];
	        this.mimeType = source["mimeType"];
	        this.byteSize = source["byteSize"];
	        this.sha256 = source["sha256"];
	        this.kind = source["kind"];
	        this.content = source["content"];
	        this.truncated = source["truncated"];
	    }
	}
	export class WorkspaceEntry {
	    name: string;
	    path: string;
	    type: string;
	    byteSize: number;
	    modifiedAt: string;
	    readable: boolean;
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.type = source["type"];
	        this.byteSize = source["byteSize"];
	        this.modifiedAt = source["modifiedAt"];
	        this.readable = source["readable"];
	    }
	}

}

