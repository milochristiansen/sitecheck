function meta()
    return {
        name        = "Exec Pass",
        description = "Runs /bin/true and verifies exit code 0",
    }
end

function check()
    local r = exec_command("/bin/true", {}, {
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
