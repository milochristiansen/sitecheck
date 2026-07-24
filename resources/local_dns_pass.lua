function meta()
    return {
        skip        = false,
        name        = "Local DNS Check",
        description = "DNS lookup via local outpost",
        notify      = {
            pass     = false,
            degraded = false,
            fail     = false,
        },
    }
end

function check()
    local r = dns_lookup("google.com", {
        timeout = 10,
    })

    if r.Error ~= nil then
        r.FailReason = r.Error
        return r
    end

    if #r.IPs == 0 then
        r.FailReason = "no IPs resolved"
        return r
    end

    r.Pass = PASS
    return r
end
