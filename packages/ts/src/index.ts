// Typed clients for the Synthwerk APIs. The types in ./gen are generated: run `just gen`.
import createClient, { type ClientOptions } from "openapi-fetch";
import type { components, paths } from "./gen/health.ts";
import type { components as IdentityComponents, paths as IdentityPaths } from "./gen/identity.ts";
import type { components as LlmComponents, paths as LlmPaths } from "./gen/llm.ts";

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
