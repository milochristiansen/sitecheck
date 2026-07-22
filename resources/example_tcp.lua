function meta()
    return {
        skip        = true,
        name        = "Example TCP",
        description = "Checks TCP connectivity to a known port",
    }
end

function check()
    local r = tcp_connect("httpbin.org", 443, {
        timeout = 10,
    })

    if r.Error ~= nil then
        r.FailReason = r.Error
        return r
    end

    r.Pass = PASS
    return r
end
