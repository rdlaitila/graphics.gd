#define _GNU_SOURCE
#include <dlfcn.h>
#include <stdio.h>
#include <stdlib.h>
#include <pthread.h>
#include <semaphore.h>
#include <stdint.h>

#define TLS_POOL_SIZE 64

/* TLS stack for nested trampoline calls (per-thread). */
__thread struct {
  long sp;
  void *stack[32];
} __tramp_ctx;

/* TLS pool: array of glibc TLS pointers, one per pool thread. */
struct tls_pool {
  void *tls_ptrs[TLS_POOL_SIZE];      /* glibc TLS pointers */
  void *tramp_ctxs[TLS_POOL_SIZE];    /* per-thread __tramp_ctx addresses */
  sem_t ready;                         /* signaled when all threads ready */
  sem_t shutdown;                      /* signaled to shut down threads */
  int count;                           /* number of threads initialized */
  pthread_mutex_t lock;
} __tls_pool;

static void *get_tls(void) {
  void *tls;
#if defined(__x86_64__)
  __asm__ volatile("mov %%fs:0, %0" : "=r"(tls));
#elif defined(__aarch64__)
  __asm__ volatile("mrs %0, tpidr_el0" : "=r"(tls));
#else
#error "unsupported architecture"
#endif
  return tls;
}

static void *pool_thread(void *arg) {
  int idx = (int)(intptr_t)arg;
  pthread_mutex_lock(&__tls_pool.lock);
  __tls_pool.tls_ptrs[idx] = get_tls();
  __tls_pool.tramp_ctxs[idx] = &__tramp_ctx;
  __tls_pool.count++;
  pthread_mutex_unlock(&__tls_pool.lock);
  sem_post(&__tls_pool.ready); /* signal this thread recorded its TLS */
  sem_wait(&__tls_pool.shutdown); /* sleep forever */
  return NULL;
}

/* On-demand glibc TCB factory: spawn a parked glibc thread and return its
   TLS pointer, so the musl side can hand fresh glibc TCBs to threads beyond
   the pre-spawned pool (no fixed cap). The musl side serializes calls, so the
   statics below need no locking. */
static sem_t __tcb_ready;
static void *__tcb_captured;
static void *tcb_thread(void *arg) {
  (void)arg;
  __tcb_captured = get_tls();
  sem_post(&__tcb_ready);
  sem_wait(&__tls_pool.shutdown); /* park forever */
  return NULL;
}
void *glibc_tcb_create(void) {
  pthread_attr_t attr;
  pthread_attr_init(&attr);
  pthread_attr_setstacksize(&attr, 16384);
  sem_init(&__tcb_ready, 0, 0);
  pthread_t t;
  int rc = pthread_create(&t, &attr, tcb_thread, NULL);
  pthread_attr_destroy(&attr);
  if (rc != 0) return NULL;
  pthread_detach(t);
  sem_wait(&__tcb_ready);
  return __tcb_captured;
}

int main(int argc, char **argv, char **envp) {
  char *ep;
  long addr;
  if (argc != 2) {
    fprintf(stderr, "%s: not intended to be run directly\n", argv[0]);
    return 1;
  }
  addr = strtol(argv[1], &ep, 10);
  if (*ep) {
    fprintf(stderr, "%s: invalid function address\n", argv[0]);
    return 2;
  }
  /* Initialize TLS pool */
  sem_init(&__tls_pool.ready, 0, 0);
  sem_init(&__tls_pool.shutdown, 0, 0);
  pthread_mutex_init(&__tls_pool.lock, NULL);
  __tls_pool.count = 0;
  /* Slot 0 is for main thread */
  __tls_pool.tls_ptrs[0] = get_tls();
  __tls_pool.tramp_ctxs[0] = &__tramp_ctx;
  __tls_pool.count = 1;
  /* Create pool threads */
  pthread_attr_t attr;
  pthread_attr_init(&attr);
  pthread_attr_setstacksize(&attr, 16384); /* minimal stack */
  int created = 0;
  for (int i = 1; i < TLS_POOL_SIZE; i++) {
    pthread_t t;
    if (pthread_create(&t, &attr, pool_thread, (void*)(intptr_t)i) == 0) {
      pthread_detach(t);
      created++;
    }
  }
  pthread_attr_destroy(&attr);
  /* Wait only for the threads that were actually created, so a failed
     pthread_create (e.g. RLIMIT_NPROC in a container) cannot hang us
     forever. Unfilled slots keep NULL TLS pointers; the musl side's
     get_thread_slot() aborts cleanly if one is ever assigned. */
  for (int i = 0; i < created; i++) sem_wait(&__tls_pool.ready);
  if (created < TLS_POOL_SIZE - 1)
    fprintf(stderr, "dlopen helper: only %d of %d TLS pool threads "
                    "created; foreign calls limited to %d threads\n",
            created, TLS_POOL_SIZE - 1, created + 1);
  return ((int (*)(void *))addr)((void *[]){
      dlopen,
      dlsym,
      dlclose,
      dlerror,
      &__tls_pool,
      glibc_tcb_create,
  });
}
