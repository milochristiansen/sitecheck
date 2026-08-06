function meta()
    return {
        skip        = true,
        name        = "SSH Daemon",
        description = "Checks that sshd.service is active and running",
    }
end

function check()
    local r = systemd_check("sshd.service")

    if r.Error ~= "" then
        r.FailReason = r.Error
        return r
    end

    if r.ActiveState == "active" and r.SubState == "running" then
        r.Pass = PASS
        return r
    end

    if r.ActiveState == "failed" then
        r.Pass = FAIL
        r.FailReason = "unit in failed state"
        return r
    end

    r.Pass = DEGRADED
    r.FailReason = r.ActiveState .. "/" .. r.SubState
    return r
end
