function meta()
    return {
        skip        = false,
        name        = "Local TCP Check",
        description = "TCP connectivity check via local outpost",
        notify      = {
            pass     = false,
            degraded = false,
            fail     = false,
        },
    }
end

function check()
    local r = tcp_connect("google.com", 443, {
        timeout = 10,
    })

    if r.Error ~= nil then
        r.FailReason = r.Error
        return r
    end

    r.Pass = PASS
    return r
end
