# Certree 시작 가이드

이 문서는 **새 평가 환경**을 설치할 때 사용합니다. 기존 서버·DB가 있으면
초기 설정을 다시 만들지 말고 [업데이트 가이드](UPDATING.md)를 사용하세요.
[English](GETTING_STARTED.md) · [한국어 README](../README.ko.md)

## 설치 전 확인

- Linux 관리 서버에 Docker Engine과 Compose v2가 있어야 합니다.
- 초기 설정 생성에는 `go.mod`에 지정된 Go 버전이 필요합니다.
- 업데이트 도구는 Python 3, Git 갱신은 Git을 사용합니다.
- 배포 대상은 현재 Nginx입니다. SSH/SFTP 또는 Linux 에이전트 경로를 선택합니다.
- 실제 서비스 연결 전 [보안 정책](../SECURITY.md)과 [검증 상태](VERIFICATION.md)를 읽으세요.

## 새로 설치하기

```sh
git clone https://github.com/polarbear138/certree.git
cd certree
go run ./cmd/setup
docker compose up -d --build
docker compose ps
```

`.env`에 생성된 초기 관리자 계정으로 로그인하고 Settings에서 비밀번호를
변경합니다. 초기 설정 생성기는 기존 `.env`를 덮어쓰지 않습니다. **기존 DB에
새 키를 만들어 연결하면 저장된 인증정보를 복호화할 수 없습니다.**

기본 접속 주소는 설치 장비의 `https://localhost:8443`입니다. Caddy가 사용하는
내부 CA의 공개 루트 인증서를 내보내 워크스테이션에 신뢰 설정을 합니다.

```sh
docker compose cp proxy:/data/caddy/pki/authorities/local/root.crt ./certree-local-root.crt
```

개인 CA 키를 내보내거나 공유하는 과정이 아닙니다. 조직에서 발급한 관리용
인증서를 사용하는 방식도 가능합니다.

## 원격 PC에서 접속하기

기본 포트 매핑은 `127.0.0.1:8443:443`입니다. 방화벽에서 8443을 열어도
Docker가 루프백에만 바인딩되어 있으면 외부에서는 접속할 수 없습니다.

1. 관리자가 사용할 호스트명과 DNS를 정합니다. 예: `certree.example.com`.
2. `compose.yaml`의 `proxy` 포트 매핑을 관리용 인터페이스에 맞춥니다.
   `8443:443`은 모든 호스트 인터페이스에 공개하므로 실제 접근 범위는 인프라
   방화벽·접근제어에서 관리자가 사용할 범위로 제한합니다.
3. `.env`를 `CERTREE_ORIGIN=https://certree.example.com:8443`으로 맞춥니다.
4. `deployments/docker/Caddyfile`을 수정합니다. 이 파일의 주소에는 호스트 포트
   8443을 붙이지 않습니다.

   ```caddyfile
   https://certree.example.com {
       tls internal
       reverse_proxy certree:8080
   }
   ```

5. 적용하고 원격 PC에서도 관리용 인증서를 신뢰하도록 설정합니다.

   ```sh
   docker compose up -d
   docker compose restart proxy
   ```

IP로만 접속하는 환경의 Caddy `default_sni` 예시는
[영문 가이드](GETTING_STARTED.md#remote-access)에 있습니다. 브라우저 주소와
`CERTREE_ORIGIN`, Caddy의 사이트 주소는 같은 대상을 가리켜야 합니다.

## 첫 인증서 배포

1. Tree Explorer에서 조직과 환경을 만듭니다.
2. Certificates에서 인증서와 일치하는 개인키가 포함된 번들을 가져옵니다.
3. 호스트명·유효기간·키 일치·신뢰 검증 결과를 확인하고 버전을 승인합니다.
4. SSH 서버와 배포 대상을 등록하거나 Settings에서 에이전트를 등록합니다.
   자세한 에이전트 설치는 [AGENT.md](AGENT.md)를 따릅니다.
5. 관리 서버에서 실제 TLS 검증 주소에 접근 가능한지 확인합니다.
   에이전트가 연결된다고 해서 관리 서버의 검증 경로도 자동으로 열리는 것은 아닙니다.
6. 단일 배포를 실행하거나 트리 체크박스로 여러 대상을 골라 Review deployment에서
   서버·승인 버전·지문·제외 사유를 확인한 뒤 실행합니다.
7. History → Deployments에서 결과를 확인합니다. 재로드만 성공한 상태와 실제
   TLS 검증까지 성공한 상태는 다릅니다.

관리 서비스의 HTTPS 인증서 신뢰와, 배포 대상 인증서의 발급기관 신뢰는 별개입니다.
사설 발급기관을 사용한다면 관리 API가 신뢰할 수 있도록 별도 번들을 읽기 전용으로
마운트하고 `CERTREE_TRUST_BUNDLE`을 설정해야 합니다.

## 화면 해석

- **Verification**: 알려진 활성 버전을 5분마다 점검한 기록입니다. 자동 재배포가 아닙니다.
- **Deployments**: 관리자가 요청한 실제 배포와 복구 기록입니다.
- **만료 바**: 마지막으로 적용·검증된 인증서 기준입니다. 새 파일 업로드만으로 바뀌지 않습니다.
- **서버 만료 요약**: 확인된 대상 중 가장 빠른 만료와 미배포·확인 불가 개수를 함께 봅니다.
- **Queued / Pending**: 각각 서버 사용을 기다리는 상태와 실행 대상으로 승인된 대기 상태입니다.
- **필터와 체크박스**: 필터 밖에 선택 대상이 남을 수 있으므로 전체 선택 개수와 안내를 확인합니다.

## 기존 데이터를 유지한 업데이트

새 ZIP은 별도 소스 폴더에 풀고 원래 설치 폴더를 보존합니다. 새 소스 루트에서:

```sh
sh scripts/update.sh --container certree-two-server-test-certree-1
```

기본 설치의 컨테이너 이름은 `certree-certree-1`일 수 있습니다. 실제 기존 앱
이름을 사용하세요. 이후 같은 Git 체크아웃에서는:

```sh
sh scripts/update.sh --pull
```

원격 저장소에 반영된 소스를 받고 앱 이미지를 재빌드합니다. DB·키·CA·프록시를
매번 다시 설정하지 않습니다. [UPDATING.md](UPDATING.md)의 프로필 보관 경로와
백업·실패 시 한계를 확인하세요.

## 접속이 안 되거나 데이터가 없어 보일 때

```sh
docker ps -a --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}'
docker compose ps
docker compose logs --tail=80 certree proxy
```

포트 충돌은 기존 프록시 점유 여부, TLS 오류는 Caddy 주소와 신뢰 설정, 빈 목록은
Compose 프로젝트와 `/data` 볼륨이 기존 환경과 같은지부터 확인합니다.
**컨테이너 재시작만으로 DB가 삭제되지는 않습니다.** 다른 프로젝트 이름으로
실행하면 별도의 DB 볼륨이 만들어질 수 있습니다.

설치 데이터에 `down -v`나 광범위한 prune 명령을 사용하지 마세요.
`.env`, 암호화 키, 개인키와 원본 민감 로그를 공개하지 마세요.
