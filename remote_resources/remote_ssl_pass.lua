function meta()
    return {
        skip        = false,
        name        = "Remote SSL Check",
        description = "SSL certificate check via remote outpost",
        notify      = {
            pass     = false,
            degraded = false,
            fail     = false,
        },
    }
end

function check()
    local r = ssl_certificate("google.com", 443, {
        timeout = 10,
    })

    if r.Error ~= nil then
        r.FailReason = r.Error
        return r
    end

    if r.DaysRemaining <= 0 then
        r.Pass = FAIL
        r.FailReason = "certificate expired"
        return r
    end

    r.Pass = PASS
    return r
end
