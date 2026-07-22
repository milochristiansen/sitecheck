function meta()
    return {
        skip        = true,
        name        = "HTTP JSON Body",
        description = "Checks a JSON endpoint and displays the response body",
    }
end

function check()
    local r = http_fetch("https://httpbin.org/json", {
        method = "GET",
        timeout = 10,
        follow_redirects = true,
    })

    if r.Error ~= nil then
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
