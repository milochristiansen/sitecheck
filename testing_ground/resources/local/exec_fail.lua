function meta()
    return {
        name        = "Exec Fail",
        description = "Runs /bin/false and verifies exit code non-zero",
        sites       = { alternate = "basic" },
    }
end

function check()
    local r = exec_command("/bin/false", {}, {
        timeout = 5,
    })

    if r.Error ~= nil and r.Error ~= "" then
        r.FailReason = r.Error
        return r
    end

    -- /bin/false exits 1 — we DELIBERATELY mark this as FAIL.
    r.Pass = FAIL
    r.FailReason = "/bin/false exited with code " .. r.ExitCode
    return r
end
