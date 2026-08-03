function meta()
    return {
        name        = "Remote Exec Check",
        description = "Exec check from remote outpost — always passes",
        sites       = { alternate = "basic" },
    }
end

function check()
    local r = exec_command("echo", {"hello from remote outpost"}, {
        timeout = 5,
    })

    if r.Error ~= nil and r.Error ~= "" then
        r.FailReason = r.Error
        return r
    end

    if r.ExitCode ~= 0 then
        r.FailReason = "expected exit 0, got " .. r.ExitCode
        return r
    end

    r.Pass = PASS
    return r
end
