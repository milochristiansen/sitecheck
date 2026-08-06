function meta()
    return {
        skip        = true,
        name        = "Example Exec",
        description = "Runs `uptime` and checks exit code.",
    }
end

function check()
    local r = exec_command("uptime", {}, {
        timeout = 10,
    })

    if r.Error ~= "" then
        r.FailReason = r.Error
        return r
    end

    if r.ExitCode ~= 0 then
        r.FailReason = "Non-zero exit: " .. r.ExitCode
        return r
    end

    r.Pass = PASS
    return r
end
