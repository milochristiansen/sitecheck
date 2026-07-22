function meta()
    return {
        name        = "Example DNS",
        description = "Checks DNS resolution for a known host",
    }
end

function check()
    local r = dns_lookup("httpbin.org", {
        timeout = 10,
    })

    if r.Error ~= nil then
        r.FailReason = r.Error
        return r
    end

    r.Pass = PASS
    return r
end
