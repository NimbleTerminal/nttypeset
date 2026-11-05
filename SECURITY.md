# Security policy and boundaries

## Reporting a vulnerability

Send security reports privately to **security@neomantra.net**. Include the
nttypeset version, Go version, operating system and kernel version, whether
workers and OS confinement were enabled, and a minimal reproduction. Please do
not put exploitable details in public GitHub issues.

## Threat model

nttypeset processes potentially hostile text, document structure, font bytes,
and images. Its resource limits and error handling reduce accidental failures
and denial of service. They do not make in-process parsing a security sandbox.
The supported isolation path runs a trusted local executable on untrusted input
through ntmediabox and ntconfine. It does not safely execute adversarial helper
binaries or defend against kernel exploits or side channels.

| Entry point | Where input is processed | Security boundary |
| --- | --- | --- |
| `Lines`, `Highlight`, `goldmark.Build`, `Engine`, `Layout`, `raster` | Calling process | Application checks and cooperative cancellation; no OS confinement |
| `RasterWorker` with `WorkerPath` | Separate worker process | ntmediabox validates IPC; ntconfine requires OS confinement unless explicitly relaxed |
| `RasterWorker` with `Client` | Wherever the caller connected it | IPC checks still apply; caller owns process confinement and termination |
| Native demos in `examples/` | Demo process | In-process rendering; they do not use `RasterWorker` |
| Browser demos | Browser's WASM runtime | Browser permissions and Booba bridge; no ntmediabox subprocess confinement |

## Worker lifecycle and OS policy

`RasterWorker` starts a fresh helper for each call unless given a `Client`.
`ntmediabox.StartRenderHelper` verifies the executable through ntconfine and
attaches a kill hook to the render client. Closing the client, cancelling an
in-flight RPC, or receiving a wire protocol violation closes the transport and
terminates and reaps the helper (including its process group on Unix).
Application-level invalid raster dimensions or layout/link metadata also close
the client. There is no fallback to parsing or rasterizing in the host.

The default RPC timeout is 15 seconds, further shortened by the caller's context.
Helper startup is synchronous: that timeout cannot interrupt binary verification
or process creation. Worker-side building and rendering receive a deadline too,
but callbacks and parsers may ignore it. The host-side timeout and kill hook are
needed to stop a stuck worker.

OS enforcement comes from the pinned ntconfine dependency:

- **Linux:** Landlock, seccomp and stage resource limits. Install a trusted
  `ntconfine-stage` beside the host executable, or pass its absolute path in
  `WorkerConfig.StagePath`. A self-staging executable must handle
  `ntmediabox.StageArg` by invoking `ntmediabox.RunStage` before normal application
  startup. Prefer a dedicated stage built with `CGO_ENABLED=0`. A missing or
  invalid stage fails startup when OS enforcement is available.
- **macOS:** Seatbelt through `sandbox-exec`. This OS facility is deprecated.
  Address-space limits are not hard memory limits on macOS; deadlines, payload
  limits and host concurrency limits remain necessary.
- **Windows and other unsupported hosts:** OS confinement is unavailable.
  Startup fails closed by default. Windows supports an explicitly unsandboxed
  subprocess fallback; arbitrary other platforms may not support helpers at all.

Keep both `WorkerConfig.AllowUnsandboxed` and
`WorkerConfig.Policy.AllowUnsandboxed` false when confinement is required.
Either can opt into fallback when the OS mechanism is unavailable. Such a child
has the user's ordinary file and network privileges; a process boundary alone
is not containment. `ntmediabox.Probe()` can describe platform support, but does
not establish that an arbitrary caller-supplied client is confined.

Empty `Policy.ReadPaths` uses `ntmediabox.DefaultLibPaths()`, which includes
system library/font locations and, on macOS, Homebrew library, Cellar and opt
paths. Empty `ExecPaths` uses the helper path plus default loader paths. Nonempty
lists **replace**, rather than extend, these defaults. No write paths are added
by nttypeset. Grant only the files the worker needs; broad directories and
`WritePaths` enlarge the boundary. Embedded fonts can reduce the need for font
filesystem access. Use explicit absolute helper paths and keep executables and
their installation directories trusted; file permission checks are not binary
signatures or protection against a compromised same-user account.

Read the pinned upstream policies for the detailed OS and wire contracts:
[ntconfine](https://github.com/NimbleTerminal/ntconfine/blob/v0.1.1/SECURITY.md)
and [ntmediabox](https://github.com/NimbleTerminal/ntmediabox/blob/v0.1.0/SECURITY.md).

## Request and response validation

Before starting a helper or copying text into an IPC payload, `RasterWorker`
checks the view and input byte limit. Scale must be finite and positive (zero
selects the default of 1); explicit pixel dimensions must be paired. Raster
arithmetic is checked before multiplying or converting large floating-point
sizes. Both host and worker enforce the raster envelope, and the worker checks
it against its own lowered pixel limit before invoking the document builder.

The pinned render protocol checks frame lengths, raster dimensions and exact
pixel byte counts before allocating returned metadata or images. Its ceilings
are 8,192 pixels per raster side, 32 Mi pixels (128 MiB RGBA), 128 MiB input and
16 MiB metadata. These are ceilings, not recommended request sizes or a bound
on total process memory. IPC buffering, JSON decoding, document construction,
fonts and rendering can hold additional allocations simultaneously. Local
preflight limit errors are `*LimitError`; worker application errors cross the
wire as text, so they do not preserve `errors.Is(err, ErrLimit)`. Host-observed
context cancellation and deadline errors retain their context error identity.

nttypeset additionally requires the requested raster size and a JSON object
containing nonnegative `extent` and `extent_x` and a boolean `done`. Link count
is bounded by the display-list ceiling; rectangles must be nonempty and inside
the raster, and block/rune positions must fit their structural limits. These
checks constrain representation; a compromised worker can still lie about the
page, its extents, link destinations or pixels. Do not use worker output to
make authorization decisions.

`Link.URL` and other document-derived strings remain untrusted. nttypeset does
not open links, fetch image destinations, execute HTML, or interpret text as
terminal commands. The host must allowlist URL schemes and destinations before
opening them, and sanitize strings before writing them to a terminal, HTML or
logs. ntmediabox sanitizes its worker error strings; it does not sanitize raw
JSON metadata or extracted URLs. JSON decoding is not URL or terminal safety.

A caller-supplied `RenderClient` stays open after a successful call, allowing
reuse. Invalid responses and transport cancellation can close it. Use
`StartRenderHelper` or `NewRenderClientWithKill` with an effective process kill
hook: `NewRenderClient` alone can close a connection but cannot terminate a
stuck parser. Serialize worker calls; in particular, the underlying client's
serialization mutex can delay cancellation of requests queued behind another
call.

## Parsing, fonts, images and resource limits

`ServeWorker` builds line documents. `ServeWorkerWithBuilder` lets an application
parse raw input inside the helper. Put Markdown parsing, normalization, image
decoding and resource loading in that callback if they need isolation. Parsing
in the host and then sending text to a worker does not protect the host parser.
The callback is trusted application code and must impose its own input, nesting
and external-resource policy and honor its context. For example,
`goldmark.Build` takes an already parsed tree and recursively walks it without
a nesting limit. Bound nesting before parsing and walking hostile Markdown.

The engine caps blocks, runs, paragraph runes, glyphs, display-list entries,
layout height, raster pixels, text size and cumulative layout time. Shape and
glyph-mask caches each have a 32 MiB ceiling. `WithLimits` and worker `Limits`
can lower these ceilings, never raise them. See [limits.go](limits.go) for the
exact values. `Lines` also caps line count; highlighting caps token count and
falls back to plain spans on lexer errors or observed cancellation.

Those checks do not bound all work before layout, every parser call, total
font-source memory, image decoding, or aggregate concurrent requests. Recoverable
font and layout panics are returned as errors, but runtime out-of-memory,
stack exhaustion, crashes and non-cooperating callbacks require process
isolation and supervision. Bound input before reading it all, limit concurrent
workers, and use tighter budgets where possible. The library does not sandbox
`FaceSource`, custom `image.Image` methods or other application callbacks when
used in-process. `Engine` and `Layout` require serialized access.

## Verification and remaining limits

The worker tests cover a real subprocess with `AllowUnsandboxed` disabled when
OS confinement is available, request rejection before document building,
invalid application metadata, and timeout-triggered kill hooks. The real
subprocess test builds a separate `CGO_ENABLED=0` helper on Linux: its stage
requires `AllThreadsSyscall`, which rejects cgo-linked binaries, including
race-enabled test binaries. The parent and in-memory tests still run under the
race detector. The subprocess test skips when confinement is unavailable; a
passing in-memory test does not prove kernel enforcement. Linux stage behavior and denial of file,
network and terminal access need deployment testing on the target kernel.
Upstream ntconfine and ntmediabox carry their own confinement and hostile-wire
tests; nttypeset does not duplicate their full suites.

`task fuzz` mutates view arithmetic, JSON response metadata, and in-memory worker
render round trips. The harness bounds JSON to 64 KiB and text/link strings to
512 bytes; it does not exercise every production payload size or OS confinement.
CI runs each target for 10 seconds; longer campaigns can use
`task fuzz FUZZ_TIME=5m FUZZ_PARALLEL=2`. Failures belong in
`testdata/fuzz/<target>/` so ordinary tests replay them.

`task ci` runs race tests, bounded fuzzing, vet/lint, native and WASM builds, browser smoke tests,
notices checks and vulnerability scans. These checks are not a security audit
or a visual review. Keep Go and dependencies patched and recheck upstream
security policies whenever the confinement or transport pins change.

### Long documents

`Chunked` retains two layouts and never relaxes a layout limit. A `ChunkSource`
is application code, running in the caller's process: it must honor cancellation
and bound its storage, parsing and assets. Holding returned chunks beyond cache
eviction retains their resources. `LineChunks` retains at most 128 MiB of UTF-8
source and indexes chunks capped at 256 lines / 64 KiB. Highlighting and layout
happen on demand under the ordinary limits; a line cannot exceed 64 KiB or the
layout's paragraph rune ceiling. These APIs do not add an OS sandbox or change
the worker protocol.
