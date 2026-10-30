// Typed clients for the Synthwerk APIs. The types in ./gen are generated: run `just gen`.
import createClient, { type ClientOptions } from "openapi-fetch";
import type { components, paths } from "./gen/health.ts";
import type { components as IdentityComponents, paths as IdentityPaths } from "./gen/identity.ts";
import type { components as LlmComponents, paths as LlmPaths } from "./gen/llm.ts";
import type { components as VisionComponents, paths as VisionPaths } from "./gen/vision.ts";

export type HealthPaths = paths;
export type Liveness = components["schemas"]["Liveness"];
export type Readiness = components["schemas"]["Readiness"];
export type ReadinessStatus = components["schemas"]["ReadinessStatus"];
export type Check = components["schemas"]["Check"];

/** Client for GET /healthz and GET /readyz of one Synthwerk service. */
export function createHealthClient(options: ClientOptions) {
  return createClient<HealthPaths>(options);
}

export type { LlmPaths };
export type Conversation = LlmComponents["schemas"]["Conversation"];
export type Message = LlmComponents["schemas"]["Message"];
export type MessageStatus = LlmComponents["schemas"]["MessageStatus"];
export type SendMessageRequest = LlmComponents["schemas"]["SendMessageRequest"];
/** One Server-Sent Event of the answer stream: `delta`, `usage`, `done` or `error`. */
export type StreamEvent = LlmComponents["schemas"]["StreamEvent"];
export type Delta = LlmComponents["schemas"]["Delta"];
export type Usage = LlmComponents["schemas"]["Usage"];
export type Done = LlmComponents["schemas"]["Done"];
/** RFC 9457 problem details with the stable `code`. Same shape in every Synthwerk API. */
export type Problem = LlmComponents["schemas"]["Problem"];

/**
 * Client for the LLM gateway (conversations). The send route answers with `text/event-stream`:
 * read it with `parseAs: "stream"` and parse the frames into `StreamEvent`.
 */
export function createLlmClient(options: ClientOptions) {
  return createClient<LlmPaths>(options);
}

export type { IdentityPaths };
export type Me = IdentityComponents["schemas"]["Me"];
export type Org = IdentityComponents["schemas"]["Org"];
export type OrgPage = IdentityComponents["schemas"]["OrgPage"];
export type Role = IdentityComponents["schemas"]["Role"];
export type App = IdentityComponents["schemas"]["App"];
export type AppPage = IdentityComponents["schemas"]["AppPage"];
export type AppOrigin = IdentityComponents["schemas"]["AppOrigin"];
/** Short-lived token for a widget on an allowed origin. Never log `access_token`. */
export type WidgetToken = IdentityComponents["schemas"]["WidgetToken"];

/** Client for the identity API (the caller, orgs, roles, apps and widget tokens). */
export function createIdentityClient(options: ClientOptions) {
  return createClient<IdentityPaths>(options);
}

export type { VisionPaths };
export type LabelSet = VisionComponents["schemas"]["LabelSet"];
export type Label = VisionComponents["schemas"]["Label"];
export type LabelTopic = VisionComponents["schemas"]["Topic"];
export type CreateLabelRequest = VisionComponents["schemas"]["CreateLabelRequest"];
export type Feedback = VisionComponents["schemas"]["Feedback"];
/** Answer of POST /v1/classifications. `correction` is null when no correction matches. */
export type Classification = VisionComponents["schemas"]["Classification"];
export type ScoredLabel = VisionComponents["schemas"]["ScoredLabel"];
export type ScoredTopic = VisionComponents["schemas"]["ScoredTopic"];
export type Correction = VisionComponents["schemas"]["Correction"];
export type ImageInfo = VisionComponents["schemas"]["ImageInfo"];
export type ModelInfo = VisionComponents["schemas"]["ModelInfo"];
/** Answer of POST /v1/images: the stored image and its generated id. */
export type VisionImage = VisionComponents["schemas"]["Image"];
/** FastAPI error body `{detail}` of vision. Branch on the status, never on `detail`. */
export type VisionError = VisionComponents["schemas"]["Error"];

/**
 * Client for synthwerk-vision. Two kinds of routes with two different bearers:
 * - `POST /v1/classifications` and `POST /v1/images`: a widget token (origin-bound, browser).
 * - The label-set admin routes: the vision admin token. Server side only, never in a browser.
 */
export function createVisionClient(options: ClientOptions) {
  return createClient<VisionPaths>(options);
}
