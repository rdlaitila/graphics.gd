/* execinfo.h shim for musl targets.
 *
 * Godot's platform/linuxbsd/crash_handler_linuxbsd.cpp includes
 * <execinfo.h> when CRASH_HANDLER_ENABLED is set, and detect.py
 * sets it unconditionally when the SCons HOST is glibc — even
 * when cross-compiling TO musl. musl doesn't ship execinfo.h,
 * so the editor build fails.
 *
 * gdnext plants this file into a shim include dir and prepends
 * it to CPATH when the target LibC is musl. Backing symbols
 * (backtrace, backtrace_symbols, backtrace_symbols_fd) live in
 * musl_execinfo.c and get baked into the merged libgodot.a, so
 * the crash handler links but produces no useful backtrace —
 * same effective behaviour as upstream's own `execinfo=no`.
 */
#pragma once

#ifdef __cplusplus
extern "C" {
#endif

int backtrace(void **buffer, int size);
char **backtrace_symbols(void *const *buffer, int size);
void backtrace_symbols_fd(void *const *buffer, int size, int fd);

#ifdef __cplusplus
}
#endif
