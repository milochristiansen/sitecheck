function meta()
    return {
        name        = "Example SSL",
        description = "Checks SSL certificate validity for a known host",
    }
end

function check()
    local r = ssl_certificate("httpbin.org", 443, {
        timeout = 10,
    })

    if r.Error ~= nil then
        r.FailReason = r.Error
        return r
    end

    if r.DaysRemaining < 0 then
        r.FailReason = "certificate expired"
        return r
    end

    if r.DaysRemaining < 30 then
        r.Pass = DEGRADED
        r.FailReason = "certificate expiring soon: " .. r.DaysRemaining .. " days"
        return r
    end

    r.Pass = PASS
    return r
end
