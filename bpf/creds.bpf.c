// SPDX-License-Identifier: GPL-2.0
//
// Credential monitoring + enforcement.
//
//   lsm/task_fix_setuid — uid escalation
//   lsm/task_fix_setgid — gid escalation
//   lsm/capset          — capability set changes
//
// Reference: vendor/tetragon/bpf/process/bpf_execve_bprm_commit_creds.c.
// Tetragon emits a richer creds-change struct; we only emit deltas.

#include "common.h"

char LICENSE[] SEC("license") = "GPL";

static __always_inline int caps_allowed(struct policy *pol, __u64 effective)
{
    return (effective & pol->forbidden_caps) == 0;
}

SEC("lsm/task_fix_setuid")
int BPF_PROG(asb_setuid, struct cred *new, const struct cred *old, int flags, int ret)
{
    if (ret != 0)
        return ret;

    __u32 pol_id = lookup_policy_id();
    struct policy *pol = lookup_policy(pol_id);
    if (!pol)
        return 0;

    __u32 new_uid = BPF_CORE_READ(new, uid.val);
    __u32 old_uid = BPF_CORE_READ(old, uid.val);

    int escalating = (new_uid == 0 && old_uid != 0);
    int verdict = !escalating ? VERDICT_ALLOW
                              : (pol->mode ? VERDICT_DENY : VERDICT_AUDIT);

    struct {
        struct event_hdr   hdr;
        struct creds_event c;
    } *evt = bpf_ringbuf_reserve(&events, sizeof(*evt), 0);
    if (evt) {
        fill_hdr(&evt->hdr, EVT_CREDS_SETUID, verdict);
        evt->c.old_id         = old_uid;
        evt->c.new_id         = new_uid;
        evt->c.cap_effective  = 0;
        bpf_ringbuf_submit(evt, 0);
    }

    if (verdict == VERDICT_DENY)
        return -1;
    return 0;
}

SEC("lsm/task_fix_setgid")
int BPF_PROG(asb_setgid, struct cred *new, const struct cred *old, int flags, int ret)
{
    if (ret != 0)
        return ret;
    __u32 pol_id = lookup_policy_id();
    if (pol_id == 0)
        return 0;

    __u32 new_gid = BPF_CORE_READ(new, gid.val);
    __u32 old_gid = BPF_CORE_READ(old, gid.val);

    struct {
        struct event_hdr   hdr;
        struct creds_event c;
    } *evt = bpf_ringbuf_reserve(&events, sizeof(*evt), 0);
    if (evt) {
        fill_hdr(&evt->hdr, EVT_CREDS_SETGID, VERDICT_AUDIT);
        evt->c.old_id         = old_gid;
        evt->c.new_id         = new_gid;
        evt->c.cap_effective  = 0;
        bpf_ringbuf_submit(evt, 0);
    }
    return 0;
}

SEC("lsm/capset")
int BPF_PROG(asb_capset, struct cred *new, const struct cred *old,
             const kernel_cap_t *effective,
             const kernel_cap_t *inheritable,
             const kernel_cap_t *permitted, int ret)
{
    if (ret != 0)
        return ret;

    __u32 pol_id = lookup_policy_id();
    struct policy *pol = lookup_policy(pol_id);
    if (!pol)
        return 0;

    __u64 eff = BPF_CORE_READ(effective, val);

    int allowed = caps_allowed(pol, eff);
    int verdict = allowed ? VERDICT_ALLOW
                          : (pol->mode ? VERDICT_DENY : VERDICT_AUDIT);

    struct {
        struct event_hdr   hdr;
        struct creds_event c;
    } *evt = bpf_ringbuf_reserve(&events, sizeof(*evt), 0);
    if (evt) {
        fill_hdr(&evt->hdr, EVT_CREDS_CAPSET, verdict);
        evt->c.old_id        = 0;
        evt->c.new_id        = 0;
        evt->c.cap_effective = eff;
        bpf_ringbuf_submit(evt, 0);
    }

    if (verdict == VERDICT_DENY)
        return -1;
    return 0;
}

SEC("lsm/bpf")
int BPF_PROG(asb_bpf, int cmd, union bpf_attr *attr, unsigned int size, int ret)
{
    if (ret != 0)
        return ret;

    __u32 pol_id = lookup_policy_id();
    struct policy *pol = lookup_policy(pol_id);
    if (!pol)
        return 0;              // unmanaged → allow (the daemon lands here)

    // A sandboxed agent has no legitimate reason to call bpf() at all:
    // the only BPF state that concerns it is its own policy, and being
    // able to reach that is precisely what we're preventing. So this is
    // an unconditional deny for any managed cgroup, not an allow-list.
    int verdict = pol->mode ? VERDICT_DENY : VERDICT_AUDIT;

    struct {
        struct event_hdr   hdr;
        struct creds_event c;
    } *evt = bpf_ringbuf_reserve(&events, sizeof(*evt), 0);
    if (evt) {
        __builtin_memset(evt, 0, sizeof(*evt));
        fill_hdr(&evt->hdr, EVT_BPF, verdict);
        // No dedicated payload for this pillar; reuse creds_event and
        // record the attempted bpf() command in old_id so the operator
        // can tell BPF_MAP_UPDATE_ELEM from BPF_PROG_LOAD.
        evt->c.old_id = (__u32)cmd;
        bpf_ringbuf_submit(evt, 0);
    }

    if (verdict == VERDICT_DENY)
        return -1;   // -EPERM
    return 0;
}