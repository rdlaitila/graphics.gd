/* execinfo stub — no-op implementations of the glibc backtrace API.
 * See musl_execinfo.h for context.
 *
 * backtrace() is called from Godot's SIGSEGV/SIGFPE/SIGILL handler,
 * so this file must stay async-signal-safe: only write(2), no
 * malloc, no fprintf, no locale-touching calls.
 */
#include <stddef.h>
#include <unistd.h>

int backtrace(void **buffer, int size) {
	(void)buffer;
	(void)size;
	static const char msg[] =
		"libgodot: backtrace unavailable on musl (rebuild with glibc for stack traces)\n";
	(void)!write(2, msg, sizeof(msg) - 1);
	return 0;
}

char **backtrace_symbols(void *const *buffer, int size) {
	(void)buffer;
	(void)size;
	return NULL;
}

void backtrace_symbols_fd(void *const *buffer, int size, int fd) {
	(void)buffer;
	(void)size;
	(void)fd;
}

