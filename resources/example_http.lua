function meta()
    return {
        skip        = true,
        name        = "Example Website",
        description = "Checks the main marketing site for 200 OK",
    }
end

function check()
    local r = http_fetch("https://httpbin.org/status/200", {
        method = "GET",
        timeout = 10,
        follow_redirects = true,
    })

    if r.Error ~= "" then
        r.FailReason = r.Error
        return r
    end

    if r.StatusCode == 200 then
        r.Pass = PASS
        return r
    end

    r.Pass = DEGRADED
    r.FailReason = "unexpected status " .. r.StatusCode
    return r
end
