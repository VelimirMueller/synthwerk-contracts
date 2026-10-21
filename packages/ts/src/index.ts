// Typed clients for the Synthwerk APIs. The types in ./gen are generated: run `just gen`.
import createClient, { type ClientOptions } from "openapi-fetch";
import type { components, paths } from "./gen/health.ts";

export type HealthPaths = paths;
export type Liveness = components["schemas"]["Liveness"];
export type Readiness = components["schemas"]["Readiness"];
export type ReadinessStatus = components["schemas"]["ReadinessStatus"];
export type Check = components["schemas"]["Check"];

/** Client for GET /healthz and GET /readyz of one Synthwerk service. */
export function createHealthClient(options: ClientOptions) {
  return createClient<HealthPaths>(options);
}
