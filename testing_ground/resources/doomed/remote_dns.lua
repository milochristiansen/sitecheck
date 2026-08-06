function meta()
    return {
        name        = "Remote DNS Check",
        description = "DNS resolution check from remote outpost",
        sites       = { alternate = "basic" },
    }
end

function check()
    local r = dns_lookup("localhost", {
        timeout = 5,
    })

    if r.Error ~= "" then
        r.FailReason = r.Error
        return r
    end

    r.Pass = PASS
    return r
end
