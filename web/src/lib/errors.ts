// What went wrong, in terms the UI can explain. Every error shown to a user
// is one of these kinds with a localized text (error.<kind>.title/.text);
// raw messages from the API or the network never reach the page.

export type ErrorKind = "notFound" | "badRequest" | "unavailable" | "internal";

/** Error from the REST gateway or the network, already classified. */
export class ApiError extends Error {
  readonly kind: ErrorKind;
  /** HTTP status; 0 when the request never got a response. */
  readonly status: number;

  // No parameter properties: node --test strips types and cannot run them.
  constructor(kind: ErrorKind, status: number, message: string) {
    super(message);
    this.name = "ApiError";
    this.kind = kind;
    this.status = status;
  }
}

// gRPC codes as the gateway reports them in the error body.
const GRPC_INVALID_ARGUMENT = 3;
const GRPC_NOT_FOUND = 5;
const GRPC_UNAVAILABLE = 14;

/** Classifies a gateway response by HTTP status and gRPC code. */
export function kindOf(status: number, grpcCode?: number): ErrorKind {
  if (status === 404 || grpcCode === GRPC_NOT_FOUND) return "notFound";
  if (status === 400 || grpcCode === GRPC_INVALID_ARGUMENT) return "badRequest";
  if (status === 0 || status === 502 || status === 503 || status === 504 || grpcCode === GRPC_UNAVAILABLE) {
    return "unavailable";
  }
  return "internal";
}

/** Any thrown value -> a kind. Unknown errors are "internal". */
export function errorKind(err: unknown): ErrorKind {
  if (err instanceof ApiError) return err.kind;
  return "internal";
}

export type Result<T> = { ok: true; data: T } | { ok: false; kind: ErrorKind };

/**
 * Runs a request and turns failures into a value, so pages render a
 * localized error state instead of throwing into a 500.
 */
export async function attempt<T>(fn: () => Promise<T>): Promise<Result<T>> {
  try {
    return { ok: true, data: await fn() };
  } catch (err) {
    return { ok: false, kind: errorKind(err) };
  }
}
